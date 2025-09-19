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

	// Serve HTML template
	authorized.GET("/", func(c *gin.Context) {
		html := `<!DOCTYPE html>
<html>
<head>
    <title>Daydream Cash Cannon</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .stats { background: #f5f5f5; padding: 20px; margin: 20px 0; }
        .button { background: #007cba; color: white; padding: 15px 30px; border: none; cursor: pointer; font-size: 16px; margin: 10px 0; }
        .button:hover { background: #005a8b; }
        .success { color: green; }
        .error { color: red; }
    </style>
</head>
<body>
    <h1>Daydream Cash Cannon</h1>
    <div class="stats">
        <h3>Statistics</h3>
        <p>Total Events: <strong>%d</strong></p>
        <p>Events with Amount Owed: <strong>%d</strong></p>
        <p>Total Amount Owed: <strong>$%.2f</strong></p>
        <p>Disbursements Created: <strong>%d</strong></p>
        <p>Processed: <strong>%d</strong></p>
        <p>Failed: <strong>%d</strong></p>
        <p>Last Run: <strong>%s</strong></p>
    </div>
    <form method="post" action="/trigger-disbursements">
        <button type="submit" class="button">Trigger Disbursements</button>
    </form>
</body>
</html>`
		lastRun := "Never"
		if !stats.LastRun.IsZero() {
			lastRun = stats.LastRun.Format("2006-01-02 15:04:05 MST")
		}
		
		formattedHTML := fmt.Sprintf(html, 
			stats.TotalEvents, 
			stats.EventsWithAmount, 
			stats.TotalAmountOwed,
			stats.DisbursementsCreated,
			stats.ProcessedCount,
			stats.FailedCount,
			lastRun)
		
		c.Header("Content-Type", "text/html")
		c.String(200, formattedHTML)
	})

	authorized.POST("/trigger-disbursements", triggerDisbursements)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	r.Run(":" + port)
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

	// Process each event
	for _, event := range processedEvents {
		err := processDisbursement(event)
		if err != nil {
			log.Printf("Error processing disbursement for event %s: %v", event.ID, err)
			stats.FailedCount++
		} else {
			stats.ProcessedCount++
		}
		stats.DisbursementsCreated++
	}

	log.Printf("Disbursement process completed. Created: %d, Processed: %d, Failed: %d", 
		stats.DisbursementsCreated, stats.ProcessedCount, stats.FailedCount)

	c.Redirect(302, "/")
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
	viewID := "viwd1z4JsMUf15DNb"

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
	notes := fmt.Sprintf("Successfully processed HCB transfer. Sent $%.2f to organization %s. Completed at %s", 
		event.Fields.AmountOwed, event.Fields.HCBEventID, time.Now().Format("2006-01-02 15:04:05 MST"))
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
			DisbursementType: "autogrant",
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

	transfer := HCBTransferRequest{
		ToOrganizationID: event.Fields.HCBEventID,
		Name:             fmt.Sprintf("Daydream signup grant %d", disbursementID),
		AmountCents:      int(event.Fields.AmountOwed * 100), // Convert to cents
	}

	jsonData, err := json.Marshal(transfer)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "https://hcb.hackclub.com/api/v4/organizations/daydream/transfers/", bytes.NewBuffer(jsonData))
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
