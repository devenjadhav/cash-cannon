package main

// Haven Cash Cannon: sends signup autogrants from the Haven HQ HCB org to each
// event's HCB org, driven by the "Owed Amount" field in the Haven Airtable base.
// Adapted from the Campfire cash cannon. Every Airtable field is addressed by
// field ID so renames in the base cannot break payouts.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

type Config struct {
	AirtableAPIKey     string
	AirtableBaseID     string
	EventsTable        string
	DisbursementsTable string
	EventsFilter       string // Airtable filterByFormula selecting eligible events
	HCBToken           string
	HCBSourceOrg       string // slug or org_ id of the org that pays (Haven HQ)
	ProgramName        string
	BasicAuthUser      string
	BasicAuthPass      string
	Port               string
}

var cfg Config

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func loadConfig() {
	cfg = Config{
		AirtableAPIKey:     os.Getenv("AIRTABLE_API_KEY"),
		AirtableBaseID:     envOr("AIRTABLE_BASE_ID", "appc9YSRmmAmLI37u"),
		EventsTable:        envOr("AIRTABLE_EVENTS_TABLE", "tbl0BQWphHe0gWEIp"),
		DisbursementsTable: envOr("AIRTABLE_DISBURSEMENTS_TABLE", "tblnDFHiIEvVPWcqH"),
		EventsFilter:       envOr("AIRTABLE_EVENTS_FILTER", "AND({Status}='Active', NOT({_Is Test}), {HCB ID}!='')"),
		HCBToken:           os.Getenv("HCB_API_TOKEN"),
		HCBSourceOrg:       envOr("HCB_SOURCE_ORG", "ysws-haven"),
		ProgramName:        envOr("PROGRAM_NAME", "Haven"),
		BasicAuthUser:      os.Getenv("BASIC_AUTH_USERNAME"),
		BasicAuthPass:      os.Getenv("BASIC_AUTH_PASSWORD"),
		Port:               envOr("PORT", "8080"),
	}
	var missing []string
	for k, v := range map[string]string{
		"AIRTABLE_API_KEY": cfg.AirtableAPIKey, "HCB_API_TOKEN": cfg.HCBToken,
		"BASIC_AUTH_USERNAME": cfg.BasicAuthUser, "BASIC_AUTH_PASSWORD": cfg.BasicAuthPass,
	} {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		log.Fatalf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
}

// Airtable field IDs (Haven base appc9YSRmmAmLI37u).
const (
	// Events table
	fEvName      = "fldedrTuI1qpSYRE5" // Name
	fEvSlug      = "fldljDpbKuJ6y0Bys" // Slug (formula)
	fEvStatus    = "fldP2oeMSijEs3FnR" // Status (single select)
	fEvHCBID     = "fldsW0bQnkXwQVzZw" // HCB ID (org_xxx)
	fEvEntitled  = "fldWsB6l9bmBFYtVY" // Entitled Autogrant Amount (formula, currency)
	fEvDisbursed = "fldQ2lZHCQ5EEcZvk" // Disbursed Autogrant Amount (rollup of processed autogrants)
	fEvOwed      = "fldEWH9DlsmNkgP6Z" // Owed Amount = entitled - disbursed

	// Disbursements table
	fDsID     = "fldRSkfLg8KsBlcnj" // ID (autonumber)
	fDsEvent  = "fldq47aa1zX3jASW9" // Event (link)
	fDsAmount = "fld5TDRXiG6LZ7hWW" // Amount (currency)
	fDsStatus = "fldoj5OzDTb1sQDG5" // Status: Pending / Processed / Failed / Rejected
	fDsType   = "fldx7nBEcHgmhhf5u" // Type: Autogrant / Miscellaneous / Loan / Withdrawal
	fDsNotes  = "fld9j1H6LHaKPmjYm" // Notes

	statusPending   = "Pending"
	statusProcessed = "Processed"
	statusFailed    = "Failed"

	typeAutogrant     = "Autogrant"
	typeWithdrawal    = "Withdrawal"
	typeMiscellaneous = "Miscellaneous"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type Event struct {
	RecordID  string
	Name      string
	Slug      string
	Status    string
	HCBID     string
	Entitled  float64
	Disbursed float64
	Owed      float64
}

type RunResult struct {
	Event    string  `json:"event"`
	HCBID    string  `json:"hcb_id"`
	Amount   float64 `json:"amount"`
	Type     string  `json:"type"`
	Status   string  `json:"status"`
	Note     string  `json:"note"`
	RecordID string  `json:"disbursement_record"`
}

type DisbursementStats struct {
	Mode                 string
	TotalEvents          int
	EventsWithAmount     int
	TotalAmount          float64
	DisbursementsCreated int
	ProcessedCount       int
	FailedCount          int
	LastRun              time.Time
	Results              []RunResult
}

var (
	stats    DisbursementStats
	statsMu  sync.RWMutex
	runMu    sync.Mutex // held for the whole duration of a payout run
	httpCli  = &http.Client{Timeout: 45 * time.Second}
	tsLayout = "2006-01-02 15:04:05 MST"
)

func now() string { return time.Now().UTC().Format(tsLayout) }

// toCents converts a dollar amount to integer cents, rounding half away from
// zero (Airtable currency fields can hold fractional cents, e.g. 1250.625).
func toCents(amount float64) int {
	return int(math.Round(amount * 100))
}

// ---------------------------------------------------------------------------
// HTTP server
// ---------------------------------------------------------------------------

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}
	loadConfig()

	r := gin.Default()
	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })

	authorized := r.Group("/", gin.BasicAuth(gin.Accounts{cfg.BasicAuthUser: cfg.BasicAuthPass}))
	authorized.GET("/", serveDashboard)
	authorized.GET("/api/preview", handlePreview)
	authorized.GET("/api/last-run", handleLastRun)
	authorized.POST("/trigger-disbursements", triggerDisbursements)
	authorized.POST("/trigger-custom-disbursements", triggerCustomDisbursements)

	log.Printf("%s cash cannon: paying from HCB org %q, Airtable base %s, listening on :%s",
		cfg.ProgramName, cfg.HCBSourceOrg, cfg.AirtableBaseID, cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

var dashboardTmpl = template.Must(template.New("dash").Parse(dashboardHTML))

func serveDashboard(c *gin.Context) {
	statsMu.RLock()
	s := stats
	statsMu.RUnlock()

	lastRun := "Never"
	if !s.LastRun.IsZero() {
		lastRun = s.LastRun.UTC().Format(tsLayout)
	}
	data := map[string]interface{}{
		"Program":   cfg.ProgramName,
		"SourceOrg": cfg.HCBSourceOrg,
		"Stats":     s,
		"LastRun":   lastRun,
		"Running":   isRunning(),
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := dashboardTmpl.Execute(c.Writer, data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleLastRun(c *gin.Context) {
	statsMu.RLock()
	defer statsMu.RUnlock()
	c.JSON(200, stats)
}

func isRunning() bool {
	if runMu.TryLock() {
		runMu.Unlock()
		return false
	}
	return true
}

type EventPreview struct {
	RecordID  string  `json:"record_id"`
	Name      string  `json:"name"`
	HCBID     string  `json:"hcb_event_id"`
	Entitled  float64 `json:"entitled"`
	Disbursed float64 `json:"disbursed"`
	Amount    float64 `json:"amount"`
	Direction string  `json:"direction"`
}

func buildPlan(events []Event, customAmount float64, custom bool) ([]EventPreview, int, int) {
	var previews []EventPreview
	grantCents, withdrawCents := 0, 0
	for _, ev := range events {
		var amt float64
		if custom {
			amt = customAmount
		} else {
			amt = ev.Owed
		}
		if toCents(amt) == 0 {
			continue
		}
		dir := "grant"
		if amt < 0 {
			dir = "withdrawal"
			withdrawCents += -toCents(amt)
		} else {
			grantCents += toCents(amt)
		}
		previews = append(previews, EventPreview{
			RecordID: ev.RecordID, Name: ev.Name, HCBID: ev.HCBID,
			Entitled: ev.Entitled, Disbursed: ev.Disbursed, Amount: amt, Direction: dir,
		})
	}
	return previews, grantCents, withdrawCents
}

func handlePreview(c *gin.Context) {
	custom := false
	customAmount := 0.0
	if s := c.Query("custom_amount"); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v <= 0 {
			c.JSON(400, gin.H{"error": "Invalid custom amount"})
			return
		}
		custom, customAmount = true, v
	}

	events, err := getAllEvents()
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to fetch events: %v", err)})
		return
	}
	previews, grantCents, withdrawCents := buildPlan(events, customAmount, custom)

	balanceCents, balErr := getHCBBalance(cfg.HCBSourceOrg)
	resp := gin.H{
		"events":            previews,
		"total_events":      len(events),
		"event_count":       len(previews),
		"total_grants":      float64(grantCents) / 100,
		"total_withdrawals": float64(withdrawCents) / 100,
		"source_org":        cfg.HCBSourceOrg,
		"running":           isRunning(),
	}
	if balErr != nil {
		resp["balance_error"] = balErr.Error()
	} else {
		resp["source_balance"] = float64(balanceCents) / 100
		resp["sufficient_funds"] = balanceCents >= grantCents
	}
	if previews == nil {
		resp["events"] = []EventPreview{}
	}
	c.JSON(200, resp)
}

func triggerDisbursements(c *gin.Context) {
	runPayout(c, false, 0)
}

func triggerCustomDisbursements(c *gin.Context) {
	amountStr := c.PostForm("custom_amount")
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		c.JSON(400, gin.H{"error": "custom_amount must be a positive number"})
		return
	}
	runPayout(c, true, amount)
}

// runPayout is the single entry point for moving money. Only one run may be in
// flight at a time; a second request while one is running gets a 409.
func runPayout(c *gin.Context, custom bool, customAmount float64) {
	if !runMu.TryLock() {
		c.JSON(409, gin.H{"error": "A disbursement run is already in progress. Wait for it to finish and refresh."})
		return
	}
	defer runMu.Unlock()

	mode := "autogrant"
	if custom {
		mode = fmt.Sprintf("custom $%.2f", customAmount)
	}
	log.Printf("=== starting %s disbursement run ===", mode)

	events, err := getAllEvents()
	if err != nil {
		log.Printf("error fetching events: %v", err)
		c.JSON(500, gin.H{"error": fmt.Sprintf("Error fetching events: %v", err)})
		return
	}
	plan, grantCents, withdrawCents := buildPlan(events, customAmount, custom)
	log.Printf("found %d eligible events, %d to pay: grants $%.2f, withdrawals $%.2f",
		len(events), len(plan), float64(grantCents)/100, float64(withdrawCents)/100)

	// Pre-flight: refuse to start a run the source org cannot fund. Partial runs
	// leave a trail of Failed rows and are confusing to reconcile.
	balanceCents, err := getHCBBalance(cfg.HCBSourceOrg)
	if err != nil {
		c.JSON(502, gin.H{"error": fmt.Sprintf("Could not read HCB balance for %s: %v", cfg.HCBSourceOrg, err)})
		return
	}
	if grantCents > balanceCents {
		msg := fmt.Sprintf("Insufficient funds: %s holds $%.2f but $%.2f is needed for grants. Top up HCB and retry.",
			cfg.HCBSourceOrg, float64(balanceCents)/100, float64(grantCents)/100)
		log.Println(msg)
		c.JSON(409, gin.H{"error": msg})
		return
	}

	s := DisbursementStats{Mode: mode, LastRun: time.Now(), TotalEvents: len(events), EventsWithAmount: len(plan),
		TotalAmount: float64(grantCents-withdrawCents) / 100}
	byID := map[string]Event{}
	for _, ev := range events {
		byID[ev.RecordID] = ev
	}

	for _, p := range plan {
		ev := byID[p.RecordID]
		dtype := typeAutogrant
		if custom {
			dtype = typeMiscellaneous
		} else if p.Amount < 0 {
			dtype = typeWithdrawal
		}
		res := processDisbursement(ev, p.Amount, dtype)
		s.DisbursementsCreated++
		if res.Status == statusProcessed {
			s.ProcessedCount++
		} else {
			s.FailedCount++
		}
		s.Results = append(s.Results, res)
	}

	statsMu.Lock()
	stats = s
	statsMu.Unlock()

	log.Printf("=== run complete: created %d, processed %d, failed %d ===", s.DisbursementsCreated, s.ProcessedCount, s.FailedCount)
	c.JSON(200, gin.H{
		"created":   s.DisbursementsCreated,
		"processed": s.ProcessedCount,
		"failed":    s.FailedCount,
		"results":   s.Results,
	})
}

// processDisbursement creates the Airtable row first (so there is always a
// record of the attempt), then moves the money, then marks the row.
func processDisbursement(ev Event, amount float64, dtype string) RunResult {
	res := RunResult{Event: ev.Name, HCBID: ev.HCBID, Amount: amount, Type: dtype}
	log.Printf("processing %s for %s (%s): $%.2f", dtype, ev.Name, ev.HCBID, amount)

	createNote := fmt.Sprintf("[%s] Created by cash cannon. Event: %s (%s), HCB org: %s, type: %s, amount: $%.2f.\n"+
		"Snapshot at creation: entitled $%.2f, disbursed $%.2f, owed $%.2f.",
		now(), ev.Name, ev.RecordID, ev.HCBID, dtype, amount, ev.Entitled, ev.Disbursed, ev.Owed)
	recID, seq, err := createDisbursement(ev.RecordID, amount, dtype, createNote)
	if err != nil {
		res.Status = statusFailed
		res.Note = fmt.Sprintf("could not create Airtable disbursement row: %v", err)
		log.Printf("  %s", res.Note)
		return res
	}
	res.RecordID = recID
	log.Printf("  created disbursement #%d (%s)", seq, recID)

	hcbResp, err := sendHCBTransfer(ev, seq, amount, dtype)
	if err != nil {
		res.Status = statusFailed
		res.Note = fmt.Sprintf("HCB transfer failed: %v", err)
		note := createNote + fmt.Sprintf("\n[%s] HCB transfer FAILED: %v", now(), err)
		if uerr := updateDisbursement(recID, statusFailed, note); uerr != nil {
			log.Printf("  could not mark row failed: %v", uerr)
			res.Note += fmt.Sprintf(" (and could not update Airtable row: %v)", uerr)
		}
		log.Printf("  %s", res.Note)
		return res
	}

	var done string
	if amount < 0 {
		done = fmt.Sprintf("[%s] HCB transfer SUCCEEDED: pulled $%.2f from %s into %s. HCB response: %s",
			now(), -amount, ev.HCBID, cfg.HCBSourceOrg, hcbResp)
	} else {
		done = fmt.Sprintf("[%s] HCB transfer SUCCEEDED: sent $%.2f from %s to %s. HCB response: %s",
			now(), amount, cfg.HCBSourceOrg, ev.HCBID, hcbResp)
	}
	if err := updateDisbursement(recID, statusProcessed, createNote+"\n"+done); err != nil {
		// Money moved but the row is still Pending: the rollup will not count it
		// and a re-run would pay again. Make this loud.
		res.Status = statusFailed
		res.Note = fmt.Sprintf("MONEY MOVED but Airtable row %s could not be marked Processed: %v. Fix the row by hand before re-running.", recID, err)
		log.Printf("  %s", res.Note)
		return res
	}
	res.Status = statusProcessed
	res.Note = done
	log.Printf("  disbursement #%d processed", seq)
	return res
}

// ---------------------------------------------------------------------------
// Airtable
// ---------------------------------------------------------------------------

func airtableURL(table string) string {
	return fmt.Sprintf("https://api.airtable.com/v0/%s/%s", cfg.AirtableBaseID, table)
}

func airtableDo(method, rawURL string, body interface{}) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, rawURL, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AirtableAPIKey)
	req.Header.Set("Content-Type", "application/json")

	// Airtable rate-limits at 5 req/s per base; retry once on 429.
	for attempt := 0; ; attempt++ {
		resp, err := httpCli.Do(req)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == 429 && attempt < 3 {
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			if rdr != nil {
				b, _ := json.Marshal(body)
				req.Body = io.NopCloser(bytes.NewReader(b))
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("airtable %s %s -> %d: %s", method, rawURL, resp.StatusCode, truncate(string(data), 500))
		}
		return data, nil
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

type airtableRecord struct {
	ID     string                 `json:"id"`
	Fields map[string]interface{} `json:"fields"`
}

func num(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

func str(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func getAllEvents() ([]Event, error) {
	var all []Event
	offset := ""
	for {
		q := url.Values{}
		q.Set("filterByFormula", cfg.EventsFilter)
		q.Set("returnFieldsByFieldId", "true")
		q.Set("pageSize", "100")
		for _, f := range []string{fEvName, fEvSlug, fEvStatus, fEvHCBID, fEvEntitled, fEvDisbursed, fEvOwed} {
			q.Add("fields[]", f)
		}
		if offset != "" {
			q.Set("offset", offset)
		}
		data, err := airtableDo("GET", airtableURL(cfg.EventsTable)+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Records []airtableRecord `json:"records"`
			Offset  string           `json:"offset"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		for _, r := range page.Records {
			all = append(all, Event{
				RecordID:  r.ID,
				Name:      str(r.Fields[fEvName]),
				Slug:      str(r.Fields[fEvSlug]),
				Status:    str(r.Fields[fEvStatus]),
				HCBID:     strings.TrimSpace(str(r.Fields[fEvHCBID])),
				Entitled:  num(r.Fields[fEvEntitled]),
				Disbursed: num(r.Fields[fEvDisbursed]),
				Owed:      num(r.Fields[fEvOwed]),
			})
		}
		if page.Offset == "" {
			return all, nil
		}
		offset = page.Offset
	}
}

func createDisbursement(eventRecordID string, amount float64, dtype, notes string) (recID string, seq int, err error) {
	body := map[string]interface{}{
		"returnFieldsByFieldId": true,
		"fields": map[string]interface{}{
			fDsEvent:  []string{eventRecordID},
			fDsAmount: amount,
			fDsStatus: statusPending,
			fDsType:   dtype,
			fDsNotes:  notes,
		},
	}
	data, err := airtableDo("POST", airtableURL(cfg.DisbursementsTable), body)
	if err != nil {
		return "", 0, err
	}
	var rec airtableRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return "", 0, err
	}
	return rec.ID, int(num(rec.Fields[fDsID])), nil
}

func updateDisbursement(recID, status, notes string) error {
	body := map[string]interface{}{
		"fields": map[string]interface{}{fDsStatus: status, fDsNotes: notes},
	}
	_, err := airtableDo("PATCH", airtableURL(cfg.DisbursementsTable)+"/"+recID, body)
	return err
}

// ---------------------------------------------------------------------------
// HCB
// ---------------------------------------------------------------------------

func hcbDo(method, path string, body interface{}) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "https://hcb.hackclub.com/api/v4"+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.HCBToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpCli.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, data, err
}

func getHCBBalance(org string) (int, error) {
	code, data, err := hcbDo("GET", "/organizations/"+org, nil)
	if err != nil {
		return 0, err
	}
	if code != 200 {
		return 0, fmt.Errorf("HCB %d: %s", code, truncate(string(data), 300))
	}
	var o struct {
		BalanceCents *int `json:"balance_cents"`
	}
	if err := json.Unmarshal(data, &o); err != nil {
		return 0, err
	}
	if o.BalanceCents == nil {
		return 0, fmt.Errorf("HCB did not return balance_cents for %s (token may lack access)", org)
	}
	return *o.BalanceCents, nil
}

type hcbTransferRequest struct {
	ToOrganizationID string `json:"to_organization_id"`
	Name             string `json:"name"`
	AmountCents      int    `json:"amount_cents"`
}

// sendHCBTransfer moves money. Positive amounts go source -> event; negative
// amounts are pulled event -> source (the token must be able to transfer out
// of the event org for that to succeed).
func sendHCBTransfer(ev Event, seq int, amount float64, dtype string) (string, error) {
	var from string
	var body hcbTransferRequest
	switch {
	case amount < 0:
		from = ev.HCBID
		body = hcbTransferRequest{ToOrganizationID: cfg.HCBSourceOrg,
			Name: fmt.Sprintf("%s signup withdrawal #%d", cfg.ProgramName, seq), AmountCents: -toCents(amount)}
	case dtype == typeMiscellaneous:
		from = cfg.HCBSourceOrg
		body = hcbTransferRequest{ToOrganizationID: ev.HCBID,
			Name: fmt.Sprintf("%s miscellaneous disbursement #%d", cfg.ProgramName, seq), AmountCents: toCents(amount)}
	default:
		from = cfg.HCBSourceOrg
		body = hcbTransferRequest{ToOrganizationID: ev.HCBID,
			Name: fmt.Sprintf("%s signup grant #%d", cfg.ProgramName, seq), AmountCents: toCents(amount)}
	}
	if body.AmountCents <= 0 {
		return "", fmt.Errorf("refusing zero-cent transfer")
	}
	code, data, err := hcbDo("POST", "/organizations/"+from+"/transfers", body)
	if err != nil {
		return "", err
	}
	if code < 200 || code >= 300 {
		return "", fmt.Errorf("HCB API error (status %d): %s", code, truncate(string(data), 500))
	}
	log.Printf("  HCB transfer ok (%d): %s", code, truncate(string(data), 300))
	return truncate(string(data), 1500), nil
}
