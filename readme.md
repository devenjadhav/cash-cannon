# Daydream Cash Cannon - IMPLEMENTATION

This Go application implements the Daydream Cash Cannon service as specified in the requirements.

## Features

✅ **Basic Authentication**: Web interface protected with username/password from environment variables  
✅ **Airtable Integration**: Reads events from the specified view and creates disbursement records  
✅ **HCB API Integration**: Sends disbursements via the HCB v4 API  
✅ **Pagination Support**: Handles 100+ events using Airtable pagination  
✅ **Comprehensive Logging**: All operations logged with detailed context  
✅ **Error Handling**: Failed disbursements marked as failed with error details in notes  
✅ **Dashboard**: Real-time statistics and status tracking  

## Environment Variables Required

```bash
AIRTABLE_API_KEY=your_airtable_api_key
AIRTABLE_BASE_ID=your_airtable_base_id  
HCB_API_TOKEN=your_hcb_api_token
BASIC_AUTH_USERNAME=admin
BASIC_AUTH_PASSWORD=your_password
PORT=8080
```

## Usage

1. Set up your environment variables (copy `.env.example` to `.env`)
2. Run the application: `./cash-cannon`
3. Access the web interface at `http://localhost:8080`
4. Use basic auth credentials to login
5. Click "Trigger Disbursements" to process all events

## Process Flow

1. Fetches all events from Airtable events table (view: `viwd1z4JsMUf15DNb`)
2. Filters events with `amount_owed > 0`
3. Creates disbursement records in Airtable with status "pending"
4. Sends transfer request to HCB API
5. Updates disbursement status to "processed" or "failed" with detailed notes

## Production Ready

- Uses production environment variables for all API calls
- Comprehensive error handling and logging
- Proper status tracking for all disbursements
- Pagination support for large datasets
- Basic authentication for security
