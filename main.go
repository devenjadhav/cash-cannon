package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type AirtableEvent struct {
	ID     string `json:"id"`
	Fields struct {
		HCBEventID  string  `json:"hcb_event_id"`
		AmountOwed  float64 `json:"amount_owed"`
		RecordID    string  `json:"record_id"`
	} `json:"fields"`
}

type AirtableEventsResponse struct {
	Records []AirtableEvent `json:"records"`
	Offset  string          `json:"offset,omitempty"`
}

type AirtableDisbursement struct {
	Fields struct {
		AssociatedEvent   []string `json:"associated_event"`
		Amount            float64  `json:"amount"`
		Status            string   `json:"status"`
		DisbursementType  string   `json:"disbursement_type"`
		Notes             string   `json:"notes"`
	} `json:"fields"`
}

type AirtableDisbursementResponse struct {
	ID     string `json:"id"`
	Fields struct {
		DisbursementID   int     `json:"disbursement_id"`
		AssociatedEvent  []string `json:"associated_event"`
		Amount           float64  `json:"amount"`
		Status           string   `json:"status"`
		DisbursementType string   `json:"disbursement_type"`
		Notes            string   `json:"notes"`
	} `json:"fields"`
}

type HCBTransferRequest struct {
	ToOrganizationID string `json:"to_organization_id"`
	Name             string `json:"name"`
	AmountCents      int    `json:"amount_cents"`
}

type DisbursementStats struct {
	TotalEvents       int
	EventsWithAmount  int
	TotalAmountOwed   float64
	DisbursementsCreated int
	ProcessedCount    int
	FailedCount       int
	LastRun           time.Time
}

var stats DisbursementStats

const dashboardHTML = `<!DOCTYPE html>
<html>
<head>
    <title>Campfire Cash Cannon</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f0f2f5; color: #1a1a2e; min-height: 100vh; }
        .container { max-width: 860px; margin: 0 auto; padding: 32px 20px; }
        h1 { font-size: 28px; font-weight: 700; margin-bottom: 4px; }
        .subtitle { color: #666; font-size: 14px; margin-bottom: 28px; }
        .card { background: #fff; border-radius: 12px; padding: 24px; margin-bottom: 20px; box-shadow: 0 1px 3px rgba(0,0,0,0.08); }
        .card h2 { font-size: 16px; font-weight: 600; margin-bottom: 16px; color: #333; }
        .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 16px; }
        .stat { text-align: center; }
        .stat .value { font-size: 28px; font-weight: 700; color: #007cba; }
        .stat .label { font-size: 12px; color: #888; margin-top: 2px; text-transform: uppercase; letter-spacing: 0.5px; }
        .stat.success .value { color: #2e7d32; }
        .stat.danger .value { color: #c62828; }
        .actions { display: flex; gap: 12px; flex-wrap: wrap; }
        .btn { padding: 12px 24px; border: none; border-radius: 8px; font-size: 15px; font-weight: 600; cursor: pointer; transition: all 0.15s; display: inline-flex; align-items: center; gap: 8px; }
        .btn:disabled { opacity: 0.5; cursor: not-allowed; }
        .btn-primary { background: #007cba; color: #fff; }
        .btn-primary:hover:not(:disabled) { background: #005f8f; }
        .btn-danger { background: #c62828; color: #fff; }
        .btn-danger:hover:not(:disabled) { background: #a51d1d; }
        .btn-ghost { background: #e8e8e8; color: #333; }
        .btn-ghost:hover:not(:disabled) { background: #d0d0d0; }
        .custom-input { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
        .custom-input label { font-weight: 600; font-size: 14px; white-space: nowrap; }
        .custom-input input { padding: 10px 14px; border: 1.5px solid #ddd; border-radius: 8px; font-size: 15px; width: 160px; }
        .custom-input input:focus { outline: none; border-color: #007cba; }
        .divider { border-top: 1px solid #eee; margin: 20px 0; }

        /* Modal overlay */
        .modal-overlay { display: none; position: fixed; inset: 0; background: rgba(0,0,0,0.45); z-index: 100; justify-content: center; align-items: center; }
        .modal-overlay.active { display: flex; }
        .modal { background: #fff; border-radius: 14px; width: 90vw; max-width: 680px; max-height: 85vh; display: flex; flex-direction: column; box-shadow: 0 20px 60px rgba(0,0,0,0.25); }
        .modal-header { padding: 20px 24px; border-bottom: 1px solid #eee; display: flex; justify-content: space-between; align-items: center; }
        .modal-header h2 { font-size: 18px; margin: 0; }
        .modal-close { background: none; border: none; font-size: 22px; cursor: pointer; color: #888; padding: 4px 8px; border-radius: 6px; }
        .modal-close:hover { background: #f0f0f0; }
        .modal-body { padding: 20px 24px; overflow-y: auto; flex: 1; }
        .modal-footer { padding: 16px 24px; border-top: 1px solid #eee; display: flex; justify-content: flex-end; gap: 10px; }

        /* Preview summary */
        .preview-summary { background: #f8f9fa; border-radius: 10px; padding: 16px 20px; margin-bottom: 16px; display: flex; gap: 32px; flex-wrap: wrap; }
        .preview-summary .item { }
        .preview-summary .item .num { font-size: 22px; font-weight: 700; }
        .preview-summary .item .lbl { font-size: 12px; color: #666; }
        .preview-summary .item.total .num { color: #007cba; }
        .preview-summary .item.warn .num { color: #c62828; }

        /* Event table */
        .event-table { width: 100%%; border-collapse: collapse; font-size: 13px; }
        .event-table th { text-align: left; padding: 8px 12px; background: #f4f4f4; font-weight: 600; color: #555; font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; }
        .event-table td { padding: 8px 12px; border-bottom: 1px solid #f0f0f0; }
        .event-table tr:last-child td { border-bottom: none; }
        .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; }
        .badge-grant { background: #e8f5e9; color: #2e7d32; }
        .badge-withdrawal { background: #fce4ec; color: #c62828; }
        .amount-positive { color: #2e7d32; font-weight: 600; }
        .amount-negative { color: #c62828; font-weight: 600; }

        /* Spinner */
        .spinner { display: inline-block; width: 18px; height: 18px; border: 2.5px solid rgba(255,255,255,0.3); border-top-color: #fff; border-radius: 50%%; animation: spin 0.6s linear infinite; }
        .spinner.dark { border-color: rgba(0,0,0,0.1); border-top-color: #007cba; }
        @keyframes spin { to { transform: rotate(360deg); } }

        /* Result banner */
        .result-banner { padding: 16px 20px; border-radius: 10px; margin-bottom: 20px; display: none; }
        .result-banner.success { background: #e8f5e9; color: #1b5e20; display: block; }
        .result-banner.error { background: #fce4ec; color: #b71c1c; display: block; }
        .result-banner h3 { font-size: 15px; margin-bottom: 4px; }
        .result-banner p { font-size: 13px; margin: 2px 0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>💸 Cash Cannon</h1>
        <p class="subtitle">Campfire disbursement dashboard</p>

        <div id="resultBanner" class="result-banner"></div>

        <div class="card">
            <h2>Last Run Statistics</h2>
            <div class="stats-grid">
                <div class="stat"><div class="value">%d</div><div class="label">Total Events</div></div>
                <div class="stat"><div class="value">%d</div><div class="label">With Amount</div></div>
                <div class="stat"><div class="value">$%.2f</div><div class="label">Total Owed</div></div>
                <div class="stat"><div class="value">%d</div><div class="label">Disbursements</div></div>
                <div class="stat success"><div class="value">%d</div><div class="label">Processed</div></div>
                <div class="stat danger"><div class="value">%d</div><div class="label">Failed</div></div>
            </div>
            <p style="font-size:12px;color:#999;margin-top:12px;">Last run: %s</p>
        </div>

        <div class="card">
            <h2>Autogrant Disbursements</h2>
            <p style="font-size:13px;color:#666;margin-bottom:16px;">Disburse the <code>amount_owed</code> to each event. Only events with a non-zero balance will be processed.</p>
            <div class="actions">
                <button class="btn btn-primary" onclick="previewDisbursements('autogrant')">
                    Preview &amp; Disburse
                </button>
            </div>
        </div>

        <div class="card">
            <h2>Custom Miscellaneous Disbursements</h2>
            <p style="font-size:13px;color:#666;margin-bottom:16px;">Send a fixed dollar amount to every event in the view.</p>
            <div class="custom-input">
                <label for="customAmount">Amount per event ($)</label>
                <input type="number" id="customAmount" step="0.01" min="0.01" placeholder="0.00">
            </div>
            <div class="actions">
                <button class="btn btn-danger" onclick="previewDisbursements('custom')">
                    Preview &amp; Disburse
                </button>
            </div>
        </div>
    </div>

    <!-- Confirmation Modal -->
    <div class="modal-overlay" id="confirmModal">
        <div class="modal">
            <div class="modal-header">
                <h2 id="modalTitle">Confirm Disbursements</h2>
                <button class="modal-close" onclick="closeModal()">&times;</button>
            </div>
            <div class="modal-body" id="modalBody">
                <div style="text-align:center;padding:40px;"><div class="spinner dark"></div><p style="margin-top:12px;color:#888;font-size:13px;">Loading preview…</p></div>
            </div>
            <div class="modal-footer" id="modalFooter" style="display:none;">
                <button class="btn btn-ghost" onclick="closeModal()">Cancel</button>
                <button class="btn btn-danger" id="confirmBtn" onclick="executeDisbursements()">
                    Confirm &amp; Send Money
                </button>
            </div>
        </div>
    </div>

    <script>
    let currentMode = '';
    let currentCustomAmount = 0;

    function previewDisbursements(mode) {
        currentMode = mode;
        document.getElementById('confirmModal').classList.add('active');
        document.getElementById('modalFooter').style.display = 'none';
        document.getElementById('modalBody').innerHTML = '<div style="text-align:center;padding:40px;"><div class="spinner dark"></div><p style="margin-top:12px;color:#888;font-size:13px;">Fetching events from Airtable…</p></div>';

        let url = '/api/preview';
        if (mode === 'custom') {
            const amt = document.getElementById('customAmount').value;
            if (!amt || parseFloat(amt) <= 0) {
                document.getElementById('modalBody').innerHTML = '<p style="color:#c62828;padding:20px;">Please enter a valid amount greater than zero.</p>';
                return;
            }
            currentCustomAmount = parseFloat(amt);
            url += '?custom_amount=' + encodeURIComponent(amt);
            document.getElementById('modalTitle').textContent = 'Confirm Custom Disbursements';
        } else {
            document.getElementById('modalTitle').textContent = 'Confirm Autogrant Disbursements';
        }

        fetch(url)
            .then(r => r.json())
            .then(data => {
                if (data.error) { throw new Error(data.error); }
                renderPreview(data);
            })
            .catch(err => {
                document.getElementById('modalBody').innerHTML = '<p style="color:#c62828;padding:20px;">Error: ' + err.message + '</p>';
            });
    }

    function renderPreview(data) {
        if (data.event_count === 0) {
            document.getElementById('modalBody').innerHTML = '<p style="padding:20px;color:#666;">No events to process. All balances are zero.</p>';
            return;
        }

        const totalAbs = data.events.reduce((s, e) => s + Math.abs(e.amount), 0);
        const grants = data.events.filter(e => e.direction === 'grant');
        const withdrawals = data.events.filter(e => e.direction === 'withdrawal');

        let html = '<div class="preview-summary">';
        html += '<div class="item"><div class="num">' + data.event_count + '</div><div class="lbl">Events</div></div>';
        html += '<div class="item total"><div class="num">$' + totalAbs.toFixed(2) + '</div><div class="lbl">Total Amount</div></div>';
        if (grants.length) html += '<div class="item"><div class="num">' + grants.length + '</div><div class="lbl">Grants</div></div>';
        if (withdrawals.length) html += '<div class="item warn"><div class="num">' + withdrawals.length + '</div><div class="lbl">Withdrawals</div></div>';
        html += '</div>';

        html += '<table class="event-table"><thead><tr><th>HCB Event ID</th><th>Amount</th><th>Type</th></tr></thead><tbody>';
        data.events.forEach(e => {
            const amtClass = e.amount >= 0 ? 'amount-positive' : 'amount-negative';
            const badge = e.direction === 'grant'
                ? '<span class="badge badge-grant">Grant</span>'
                : '<span class="badge badge-withdrawal">Withdrawal</span>';
            html += '<tr><td>' + e.hcb_event_id + '</td><td class="' + amtClass + '">$' + Math.abs(e.amount).toFixed(2) + '</td><td>' + badge + '</td></tr>';
        });
        html += '</tbody></table>';

        document.getElementById('modalBody').innerHTML = html;
        document.getElementById('modalFooter').style.display = 'flex';
    }

    function executeDisbursements() {
        const btn = document.getElementById('confirmBtn');
        btn.disabled = true;
        btn.innerHTML = '<span class="spinner"></span> Processing…';

        let url, body;
        if (currentMode === 'custom') {
            url = '/trigger-custom-disbursements';
            body = 'custom_amount=' + encodeURIComponent(currentCustomAmount);
        } else {
            url = '/trigger-disbursements';
            body = '';
        }

        fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: body
        })
        .then(r => r.json())
        .then(result => {
            closeModal();
            showResult(result);
        })
        .catch(err => {
            closeModal();
            showResult({ error: err.message });
        });
    }

    function showResult(result) {
        const banner = document.getElementById('resultBanner');
        if (result.error) {
            banner.className = 'result-banner error';
            banner.innerHTML = '<h3>Disbursement Failed</h3><p>' + result.error + '</p>';
        } else {
            banner.className = 'result-banner success';
            banner.innerHTML = '<h3>Disbursements Complete</h3>'
                + '<p>Created: ' + result.created + ' · Processed: ' + result.processed + ' · Failed: ' + result.failed + '</p>';
        }
        setTimeout(() => { location.reload(); }, 3000);
    }

    function closeModal() {
        document.getElementById('confirmModal').classList.remove('active');
        const btn = document.getElementById('confirmBtn');
        btn.disabled = false;
        btn.innerHTML = 'Confirm &amp; Send Money';
    }
    </script>
</body>
</html>`

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables")
	}

	r := gin.Default()

	// Basic Auth middleware
	authorized := r.Group("/", gin.BasicAuth(gin.Accounts{
		os.Getenv("BASIC_AUTH_USERNAME"): os.Getenv("BASIC_AUTH_PASSWORD"),
	}))

	authorized.GET("/", serveDashboard)
	authorized.GET("/api/preview", handlePreview)
	authorized.POST("/trigger-disbursements", triggerDisbursements)
	authorized.POST("/trigger-custom-disbursements", triggerCustomDisbursements)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	r.Run(":" + port)
}

func serveDashboard(c *gin.Context) {
	lastRun := "Never"
	if !stats.LastRun.IsZero() {
		lastRun = stats.LastRun.Format("2006-01-02 15:04:05 MST")
	}

	html := fmt.Sprintf(dashboardHTML,
		stats.TotalEvents,
		stats.EventsWithAmount,
		stats.TotalAmountOwed,
		stats.DisbursementsCreated,
		stats.ProcessedCount,
		stats.FailedCount,
		lastRun)

	c.Header("Content-Type", "text/html")
	c.String(200, html)
}

func handlePreview(c *gin.Context) {
	customAmountStr := c.Query("custom_amount")

	events, err := getAllEvents()
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to fetch events: %v", err)})
		return
	}

	type EventPreview struct {
		RecordID   string  `json:"record_id"`
		HCBEventID string  `json:"hcb_event_id"`
		Amount     float64 `json:"amount"`
		Direction  string  `json:"direction"`
	}

	var previews []EventPreview
	totalAmount := 0.0

	if customAmountStr != "" {
		customAmount, err := strconv.ParseFloat(customAmountStr, 64)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid custom amount"})
			return
		}
		for _, event := range events {
			previews = append(previews, EventPreview{
				RecordID:   event.ID,
				HCBEventID: event.Fields.HCBEventID,
				Amount:     customAmount,
				Direction:  "grant",
			})
			totalAmount += customAmount
		}
	} else {
		for _, event := range events {
			if event.Fields.AmountOwed != 0 {
				direction := "grant"
				if event.Fields.AmountOwed < 0 {
					direction = "withdrawal"
				}
				previews = append(previews, EventPreview{
					RecordID:   event.ID,
					HCBEventID: event.Fields.HCBEventID,
					Amount:     event.Fields.AmountOwed,
					Direction:  direction,
				})
				totalAmount += event.Fields.AmountOwed
			}
		}
	}

	c.JSON(200, gin.H{
		"events":       previews,
		"total_events": len(events),
		"total_amount": totalAmount,
		"event_count":  len(previews),
	})
}

func triggerDisbursements(c *gin.Context) {
	log.Println("Starting disbursement process...")
	
	stats = DisbursementStats{}
	stats.LastRun = time.Now()
	
	// Get all events from Airtable
	events, err := getAllEvents()
	if err != nil {
		log.Printf("Error fetching events: %v", err)
		c.String(500, "Error fetching events: %v", err)
		return
	}

	stats.TotalEvents = len(events)
	log.Printf("Found %d total events", len(events))

	var processedEvents []AirtableEvent
	for _, event := range events {
		if event.Fields.AmountOwed > 0 {
			processedEvents = append(processedEvents, event)
			stats.TotalAmountOwed += event.Fields.AmountOwed
		}
	}

	stats.EventsWithAmount = len(processedEvents)
	log.Printf("Found %d events with amount owed > 0, total amount: $%.2f", len(processedEvents), stats.TotalAmountOwed)

	// Process each event including negative amounts
	for _, event := range events {
		if event.Fields.AmountOwed != 0 { // Process both positive and negative amounts
			err := processDisbursement(event)
			if err != nil {
				log.Printf("Error processing disbursement for event %s: %v", event.ID, err)
				stats.FailedCount++
			} else {
				stats.ProcessedCount++
			}
			stats.DisbursementsCreated++
		}
	}

	log.Printf("Disbursement process completed. Created: %d, Processed: %d, Failed: %d", 
		stats.DisbursementsCreated, stats.ProcessedCount, stats.FailedCount)

	c.JSON(200, gin.H{
		"created":   stats.DisbursementsCreated,
		"processed": stats.ProcessedCount,
		"failed":    stats.FailedCount,
	})
}

func triggerCustomDisbursements(c *gin.Context) {
	log.Println("Starting custom disbursement process...")
	
	customAmountStr := c.PostForm("custom_amount")
	if customAmountStr == "" {
		c.String(400, "Custom amount is required")
		return
	}
	
	var customAmount float64
	_, err := fmt.Sscanf(customAmountStr, "%f", &customAmount)
	if err != nil {
		c.String(400, "Invalid amount format: %v", err)
		return
	}
	
	// Get all events from Airtable
	events, err := getAllEvents()
	if err != nil {
		log.Printf("Error fetching events: %v", err)
		c.String(500, "Error fetching events: %v", err)
		return
	}
	
	log.Printf("Found %d events for custom disbursement of $%.2f each", len(events), customAmount)
	
	processedCount := 0
	failedCount := 0
	
	// Process each event with custom amount
	for _, event := range events {
		err := processCustomDisbursement(event, customAmount)
		if err != nil {
			log.Printf("Error processing custom disbursement for event %s: %v", event.ID, err)
			failedCount++
		} else {
			processedCount++
		}
	}
	
	log.Printf("Custom disbursement process completed. Processed: %d, Failed: %d", processedCount, failedCount)

	c.JSON(200, gin.H{
		"created":   processedCount + failedCount,
		"processed": processedCount,
		"failed":    failedCount,
	})
}

func getAllEvents() ([]AirtableEvent, error) {
	var allEvents []AirtableEvent
	offset := ""

	for {
		events, nextOffset, err := getEventsPage(offset)
		if err != nil {
			return nil, err
		}

		allEvents = append(allEvents, events...)

		if nextOffset == "" {
			break
		}
		offset = nextOffset
	}

	return allEvents, nil
}

func getEventsPage(offset string) ([]AirtableEvent, string, error) {
	baseID := os.Getenv("AIRTABLE_BASE_ID")
	apiKey := os.Getenv("AIRTABLE_API_KEY")
	viewID := "viwjvoyfA2Cgc4XE4"

	url := fmt.Sprintf("https://api.airtable.com/v0/%s/events?view=%s", baseID, viewID)
	if offset != "" {
		url += "&offset=" + offset
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("airtable API error: %s", string(body))
	}

	var response AirtableEventsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, "", err
	}

	return response.Records, response.Offset, nil
}

func processDisbursement(event AirtableEvent) error {
	log.Printf("Processing disbursement for event %s (HCB ID: %s, Amount: $%.2f)", 
		event.ID, event.Fields.HCBEventID, event.Fields.AmountOwed)

	// Create disbursement in Airtable
	disbursement, err := createDisbursement(event)
	if err != nil {
		return fmt.Errorf("failed to create disbursement: %v", err)
	}

	log.Printf("Created disbursement %d for event %s", 
		disbursement.Fields.DisbursementID, event.ID)

	// Send to HCB
	err = sendHCBTransfer(event, disbursement.Fields.DisbursementID)
	if err != nil {
		// Update disbursement as failed
		notes := fmt.Sprintf("HCB transfer failed: %v. Created at %s", err, time.Now().Format("2006-01-02 15:04:05 MST"))
		updateErr := updateDisbursementStatus(disbursement.ID, "failed", notes)
		if updateErr != nil {
			log.Printf("Failed to update disbursement status: %v", updateErr)
		}
		return fmt.Errorf("HCB transfer failed: %v", err)
	}

	// Update disbursement as processed
	var notes string
	if event.Fields.AmountOwed < 0 {
		notes = fmt.Sprintf("Successfully processed HCB withdrawal. Received $%.2f from organization %s. Completed at %s", 
			-event.Fields.AmountOwed, event.Fields.HCBEventID, time.Now().Format("2006-01-02 15:04:05 MST"))
	} else {
		notes = fmt.Sprintf("Successfully processed HCB transfer. Sent $%.2f to organization %s. Completed at %s", 
			event.Fields.AmountOwed, event.Fields.HCBEventID, time.Now().Format("2006-01-02 15:04:05 MST"))
	}
	err = updateDisbursementStatus(disbursement.ID, "processed", notes)
	if err != nil {
		log.Printf("Failed to update disbursement status: %v", err)
		return err
	}

	log.Printf("Successfully completed disbursement %d", disbursement.Fields.DisbursementID)
	return nil
}

func createDisbursement(event AirtableEvent) (*AirtableDisbursementResponse, error) {
	baseID := os.Getenv("AIRTABLE_BASE_ID")
	apiKey := os.Getenv("AIRTABLE_API_KEY")

	disbursementType := "autogrant"
	if event.Fields.AmountOwed < 0 {
		disbursementType = "withdrawal"
	}

	disbursement := AirtableDisbursement{
		Fields: struct {
			AssociatedEvent   []string `json:"associated_event"`
			Amount            float64  `json:"amount"`
			Status            string   `json:"status"`
			DisbursementType  string   `json:"disbursement_type"`
			Notes             string   `json:"notes"`
		}{
			AssociatedEvent:  []string{event.ID},
			Amount:           event.Fields.AmountOwed,
			Status:           "pending",
			DisbursementType: disbursementType,
			Notes:            fmt.Sprintf("Created for event %s at %s", event.ID, time.Now().Format("2006-01-02 15:04:05 MST")),
		},
	}

	jsonData, err := json.Marshal(disbursement)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.airtable.com/v0/%s/disbursements", baseID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("airtable API error: %s", string(body))
	}

	var response AirtableDisbursementResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func sendHCBTransfer(event AirtableEvent, disbursementID int) error {
	token := os.Getenv("HCB_API_TOKEN")

	var transfer HCBTransferRequest
	var url string

	if event.Fields.AmountOwed < 0 {
		// Negative amount - withdrawal from event to Campfire
		transfer = HCBTransferRequest{
			ToOrganizationID: "campfire",
			Name:             fmt.Sprintf("Campfire signup withdrawal ID %d", disbursementID),
			AmountCents:      int(-event.Fields.AmountOwed * 100), // Make positive for transfer amount
		}
		url = fmt.Sprintf("https://hcb.hackclub.com/api/v4/organizations/%s/transfers/", event.Fields.HCBEventID)
	} else {
		// Positive amount - grant from Campfire to event
		transfer = HCBTransferRequest{
			ToOrganizationID: event.Fields.HCBEventID,
			Name:             fmt.Sprintf("Campfire signup grant ID %d", disbursementID),
			AmountCents:      int(event.Fields.AmountOwed * 100), // Convert to cents
		}
		url = "https://hcb.hackclub.com/api/v4/organizations/campfire/transfers/"
	}

	jsonData, err := json.Marshal(transfer)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HCB API error (status %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("HCB transfer successful: %s", string(body))
	return nil
}

func updateDisbursementStatus(disbursementID, status, notes string) error {
	baseID := os.Getenv("AIRTABLE_BASE_ID")
	apiKey := os.Getenv("AIRTABLE_API_KEY")

	update := map[string]interface{}{
		"fields": map[string]interface{}{
			"status": status,
			"notes":  notes,
		},
	}

	jsonData, err := json.Marshal(update)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.airtable.com/v0/%s/disbursements/%s", baseID, disbursementID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("airtable API error updating disbursement: %s", string(body))
	}

	return nil
}

func processCustomDisbursement(event AirtableEvent, customAmount float64) error {
	log.Printf("Processing custom disbursement for event %s (HCB ID: %s, Custom Amount: $%.2f)", 
		event.ID, event.Fields.HCBEventID, customAmount)

	// Create disbursement in Airtable with custom amount
	disbursement, err := createCustomDisbursement(event, customAmount)
	if err != nil {
		return fmt.Errorf("failed to create custom disbursement: %v", err)
	}

	log.Printf("Created custom disbursement %d for event %s", 
		disbursement.Fields.DisbursementID, event.ID)

	// Send to HCB with custom amount
	err = sendCustomHCBTransfer(event, disbursement.Fields.DisbursementID, customAmount)
	if err != nil {
		// Update disbursement as failed
		notes := fmt.Sprintf("HCB custom transfer failed: %v. Created at %s", err, time.Now().Format("2006-01-02 15:04:05 MST"))
		updateErr := updateDisbursementStatus(disbursement.ID, "failed", notes)
		if updateErr != nil {
			log.Printf("Failed to update disbursement status: %v", updateErr)
		}
		return fmt.Errorf("HCB custom transfer failed: %v", err)
	}

	// Update disbursement as processed
	notes := fmt.Sprintf("Successfully processed custom HCB transfer. Sent $%.2f to organization %s. Completed at %s", 
		customAmount, event.Fields.HCBEventID, time.Now().Format("2006-01-02 15:04:05 MST"))
	err = updateDisbursementStatus(disbursement.ID, "processed", notes)
	if err != nil {
		log.Printf("Failed to update disbursement status: %v", err)
		return err
	}

	log.Printf("Successfully completed custom disbursement %d", disbursement.Fields.DisbursementID)
	return nil
}

func createCustomDisbursement(event AirtableEvent, customAmount float64) (*AirtableDisbursementResponse, error) {
	baseID := os.Getenv("AIRTABLE_BASE_ID")
	apiKey := os.Getenv("AIRTABLE_API_KEY")

	disbursement := AirtableDisbursement{
		Fields: struct {
			AssociatedEvent   []string `json:"associated_event"`
			Amount            float64  `json:"amount"`
			Status            string   `json:"status"`
			DisbursementType  string   `json:"disbursement_type"`
			Notes             string   `json:"notes"`
		}{
			AssociatedEvent:  []string{event.ID},
			Amount:           customAmount,
			Status:           "pending",
			DisbursementType: "miscellaneous",
			Notes:            fmt.Sprintf("Custom disbursement created for event %s at %s", event.ID, time.Now().Format("2006-01-02 15:04:05 MST")),
		},
	}

	jsonData, err := json.Marshal(disbursement)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.airtable.com/v0/%s/disbursements", baseID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("airtable API error: %s", string(body))
	}

	var response AirtableDisbursementResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func sendCustomHCBTransfer(event AirtableEvent, disbursementID int, customAmount float64) error {
	token := os.Getenv("HCB_API_TOKEN")

	transfer := HCBTransferRequest{
		ToOrganizationID: event.Fields.HCBEventID,
		Name:             fmt.Sprintf("Campfire miscellaneous disbursement %d", disbursementID),
		AmountCents:      int(customAmount * 100), // Convert to cents
	}
	url := "https://hcb.hackclub.com/api/v4/organizations/campfire/transfers/"

	jsonData, err := json.Marshal(transfer)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HCB API error (status %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("HCB custom transfer successful: %s", string(body))
	return nil
}
