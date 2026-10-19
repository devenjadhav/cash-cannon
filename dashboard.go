package main

// Dashboard served at "/". Rendered with html/template; the JS below talks to
// /api/preview, /trigger-disbursements and /trigger-custom-disbursements.
const dashboardHTML = `<!DOCTYPE html>
<html>
<head>
    <title>{{.Program}} Cash Cannon</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f0f2f5; color: #1a1a2e; min-height: 100vh; }
        .container { max-width: 900px; margin: 0 auto; padding: 32px 20px; }
        h1 { font-size: 28px; font-weight: 700; margin-bottom: 4px; }
        .subtitle { color: #666; font-size: 14px; margin-bottom: 28px; }
        code { background: #eef0f3; padding: 1px 5px; border-radius: 4px; font-size: 12px; }
        .card { background: #fff; border-radius: 12px; padding: 24px; margin-bottom: 20px; box-shadow: 0 1px 3px rgba(0,0,0,0.08); }
        .card h2 { font-size: 16px; font-weight: 600; margin-bottom: 16px; color: #333; }
        .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 16px; }
        .stat { text-align: center; }
        .stat .value { font-size: 26px; font-weight: 700; color: #007cba; }
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
        .modal-overlay { display: none; position: fixed; inset: 0; background: rgba(0,0,0,0.45); z-index: 100; justify-content: center; align-items: center; }
        .modal-overlay.active { display: flex; }
        .modal { background: #fff; border-radius: 14px; width: 92vw; max-width: 760px; max-height: 85vh; display: flex; flex-direction: column; box-shadow: 0 20px 60px rgba(0,0,0,0.25); }
        .modal-header { padding: 20px 24px; border-bottom: 1px solid #eee; display: flex; justify-content: space-between; align-items: center; }
        .modal-header h2 { font-size: 18px; margin: 0; }
        .modal-close { background: none; border: none; font-size: 22px; cursor: pointer; color: #888; padding: 4px 8px; border-radius: 6px; }
        .modal-close:hover { background: #f0f0f0; }
        .modal-body { padding: 20px 24px; overflow-y: auto; flex: 1; }
        .modal-footer { padding: 16px 24px; border-top: 1px solid #eee; display: flex; justify-content: flex-end; gap: 10px; }
        .preview-summary { background: #f8f9fa; border-radius: 10px; padding: 16px 20px; margin-bottom: 16px; display: flex; gap: 28px; flex-wrap: wrap; }
        .preview-summary .item .num { font-size: 22px; font-weight: 700; }
        .preview-summary .item .lbl { font-size: 12px; color: #666; }
        .preview-summary .item.total .num { color: #007cba; }
        .preview-summary .item.warn .num { color: #c62828; }
        .preview-summary .item.ok .num { color: #2e7d32; }
        .notice { padding: 12px 16px; border-radius: 8px; margin-bottom: 14px; font-size: 13px; }
        .notice.bad { background: #fce4ec; color: #b71c1c; }
        .notice.good { background: #e8f5e9; color: #1b5e20; }
        .event-table { width: 100%; border-collapse: collapse; font-size: 13px; }
        .event-table th { text-align: left; padding: 8px 10px; background: #f4f4f4; font-weight: 600; color: #555; font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; }
        .event-table td { padding: 7px 10px; border-bottom: 1px solid #f0f0f0; vertical-align: top; }
        .event-table tr:last-child td { border-bottom: none; }
        .event-table td.num { text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
        .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; }
        .badge-grant { background: #e8f5e9; color: #2e7d32; }
        .badge-withdrawal { background: #fce4ec; color: #c62828; }
        .badge-ok { background: #e8f5e9; color: #2e7d32; }
        .badge-fail { background: #fce4ec; color: #c62828; }
        .amount-positive { color: #2e7d32; font-weight: 600; }
        .amount-negative { color: #c62828; font-weight: 600; }
        .spinner { display: inline-block; width: 18px; height: 18px; border: 2.5px solid rgba(255,255,255,0.3); border-top-color: #fff; border-radius: 50%; animation: spin 0.6s linear infinite; }
        .spinner.dark { border-color: rgba(0,0,0,0.1); border-top-color: #007cba; }
        @keyframes spin { to { transform: rotate(360deg); } }
        .result-banner { padding: 16px 20px; border-radius: 10px; margin-bottom: 20px; display: none; }
        .result-banner.success { background: #e8f5e9; color: #1b5e20; display: block; }
        .result-banner.error { background: #fce4ec; color: #b71c1c; display: block; }
        .result-banner h3 { font-size: 15px; margin-bottom: 4px; }
        .result-banner p { font-size: 13px; margin: 2px 0; }
        .muted { font-size: 12px; color: #999; margin-top: 12px; }
        details summary { cursor: pointer; font-size: 13px; color: #007cba; margin-top: 12px; }
        .note-cell { max-width: 420px; white-space: pre-wrap; font-size: 11px; color: #555; }
    </style>
</head>
<body>
    <div class="container">
        <h1>💸 {{.Program}} Cash Cannon</h1>
        <p class="subtitle">Pays each event's <code>Owed Amount</code> from HCB org <code>{{.SourceOrg}}</code>. Every payout is logged in the Airtable <em>Disbursements</em> table.</p>

        <div id="resultBanner" class="result-banner"></div>
        {{if .Running}}<div class="result-banner error" style="display:block"><h3>A run is in progress</h3><p>Refresh in a minute. Do not start another run.</p></div>{{end}}

        <div class="card">
            <h2>Last run{{if .Stats.Mode}} ({{.Stats.Mode}}){{end}}</h2>
            <div class="stats-grid">
                <div class="stat"><div class="value">{{.Stats.TotalEvents}}</div><div class="label">Eligible events</div></div>
                <div class="stat"><div class="value">{{.Stats.EventsWithAmount}}</div><div class="label">With amount</div></div>
                <div class="stat"><div class="value">${{printf "%.2f" .Stats.TotalAmount}}</div><div class="label">Net amount</div></div>
                <div class="stat"><div class="value">{{.Stats.DisbursementsCreated}}</div><div class="label">Disbursements</div></div>
                <div class="stat success"><div class="value">{{.Stats.ProcessedCount}}</div><div class="label">Processed</div></div>
                <div class="stat danger"><div class="value">{{.Stats.FailedCount}}</div><div class="label">Failed</div></div>
            </div>
            <p class="muted">Last run: {{.LastRun}} (stats reset when the app restarts; the Airtable table is the source of truth)</p>
            {{if .Stats.Results}}
            <details><summary>Show {{len .Stats.Results}} results from the last run</summary>
            <table class="event-table" style="margin-top:10px"><thead><tr><th>Event</th><th>HCB org</th><th>Amount</th><th>Type</th><th>Status</th><th>Note</th></tr></thead><tbody>
            {{range .Stats.Results}}<tr><td>{{.Event}}</td><td><code>{{.HCBID}}</code></td><td class="num">${{printf "%.2f" .Amount}}</td><td>{{.Type}}</td>
                <td>{{if eq .Status "Processed"}}<span class="badge badge-ok">Processed</span>{{else}}<span class="badge badge-fail">{{.Status}}</span>{{end}}</td>
                <td class="note-cell">{{.Note}}</td></tr>{{end}}
            </tbody></table></details>
            {{end}}
        </div>

        <div class="card">
            <h2>Autogrant disbursements</h2>
            <p style="font-size:13px;color:#666;margin-bottom:16px;">Sends each active, non-test event with an HCB org its <code>Owed Amount</code> (entitled minus already disbursed). Negative balances are pulled back as withdrawals. The run is refused if <code>{{.SourceOrg}}</code> cannot cover the grants.</p>
            <div class="actions">
                <button class="btn btn-primary" onclick="previewDisbursements('autogrant')">Preview &amp; disburse</button>
            </div>
        </div>

        <div class="card">
            <h2>Custom miscellaneous disbursement</h2>
            <p style="font-size:13px;color:#666;margin-bottom:16px;">Sends a fixed amount to <strong>every</strong> eligible event, regardless of what is owed. Logged as type <em>Miscellaneous</em>, so it does not reduce Owed Amount.</p>
            <div class="custom-input">
                <label for="customAmount">Amount per event ($)</label>
                <input type="number" id="customAmount" step="0.01" min="0.01" placeholder="0.00">
            </div>
            <div class="actions">
                <button class="btn btn-danger" onclick="previewDisbursements('custom')">Preview &amp; disburse</button>
            </div>
        </div>
    </div>

    <div class="modal-overlay" id="confirmModal">
        <div class="modal">
            <div class="modal-header">
                <h2 id="modalTitle">Confirm disbursements</h2>
                <button class="modal-close" onclick="closeModal()">&times;</button>
            </div>
            <div class="modal-body" id="modalBody"></div>
            <div class="modal-footer" id="modalFooter" style="display:none;">
                <button class="btn btn-ghost" onclick="closeModal()">Cancel</button>
                <button class="btn btn-danger" id="confirmBtn" onclick="executeDisbursements()">Confirm &amp; send money</button>
            </div>
        </div>
    </div>

    <script>
    let currentMode = '';
    let currentCustomAmount = 0;
    const money = n => '$' + Math.abs(n).toFixed(2);
    const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
    const spinner = msg => '<div style="text-align:center;padding:40px;"><div class="spinner dark"></div><p style="margin-top:12px;color:#888;font-size:13px;">' + msg + '</p></div>';

    function previewDisbursements(mode) {
        currentMode = mode;
        document.getElementById('confirmModal').classList.add('active');
        document.getElementById('modalFooter').style.display = 'none';
        document.getElementById('modalBody').innerHTML = spinner('Fetching events from Airtable and the HCB balance…');

        let url = '/api/preview';
        if (mode === 'custom') {
            const amt = document.getElementById('customAmount').value;
            if (!amt || parseFloat(amt) <= 0) {
                document.getElementById('modalBody').innerHTML = '<p style="color:#c62828;padding:20px;">Enter an amount greater than zero.</p>';
                return;
            }
            currentCustomAmount = parseFloat(amt);
            url += '?custom_amount=' + encodeURIComponent(amt);
            document.getElementById('modalTitle').textContent = 'Confirm custom disbursements';
        } else {
            document.getElementById('modalTitle').textContent = 'Confirm autogrant disbursements';
        }

        fetch(url)
            .then(r => r.json())
            .then(data => { if (data.error) throw new Error(data.error); renderPreview(data); })
            .catch(err => { document.getElementById('modalBody').innerHTML = '<p style="color:#c62828;padding:20px;">Error: ' + esc(err.message) + '</p>'; });
    }

    function renderPreview(data) {
        if (data.event_count === 0) {
            document.getElementById('modalBody').innerHTML = '<p style="padding:20px;color:#666;">Nothing to pay. Every eligible event has a zero balance.</p>';
            return;
        }
        const grants = data.events.filter(e => e.direction === 'grant');
        const withdrawals = data.events.filter(e => e.direction === 'withdrawal');
        let html = '';
        if (data.running) html += '<div class="notice bad">A run is already in progress. Wait for it to finish.</div>';
        if (data.balance_error) {
            html += '<div class="notice bad">Could not read the HCB balance of ' + esc(data.source_org) + ': ' + esc(data.balance_error) + '. The run will be refused until this works.</div>';
        } else if (!data.sufficient_funds) {
            html += '<div class="notice bad">Insufficient funds: ' + esc(data.source_org) + ' holds ' + money(data.source_balance) + ' but ' + money(data.total_grants) + ' is needed. Top up HCB first; the run will be refused.</div>';
        } else {
            html += '<div class="notice good">' + esc(data.source_org) + ' holds ' + money(data.source_balance) + '; ' + money(data.total_grants) + ' will be sent, leaving ' + money(data.source_balance - data.total_grants) + '.</div>';
        }
        html += '<div class="preview-summary">';
        html += '<div class="item"><div class="num">' + data.event_count + '</div><div class="lbl">Events</div></div>';
        html += '<div class="item total"><div class="num">' + money(data.total_grants) + '</div><div class="lbl">Grants (' + grants.length + ')</div></div>';
        if (withdrawals.length) html += '<div class="item warn"><div class="num">' + money(data.total_withdrawals) + '</div><div class="lbl">Withdrawals (' + withdrawals.length + ')</div></div>';
        if (data.source_balance != null) html += '<div class="item ' + (data.sufficient_funds ? 'ok' : 'warn') + '"><div class="num">' + money(data.source_balance) + '</div><div class="lbl">HCB balance</div></div>';
        html += '</div>';

        html += '<table class="event-table"><thead><tr><th>Event</th><th>HCB org</th><th style="text-align:right">Entitled</th><th style="text-align:right">Disbursed</th><th style="text-align:right">Amount</th><th>Type</th></tr></thead><tbody>';
        data.events.forEach(e => {
            const amtClass = e.amount >= 0 ? 'amount-positive' : 'amount-negative';
            const badge = e.direction === 'grant' ? '<span class="badge badge-grant">Grant</span>' : '<span class="badge badge-withdrawal">Withdrawal</span>';
            html += '<tr><td>' + esc(e.name) + '</td><td><code>' + esc(e.hcb_event_id) + '</code></td><td class="num">' + money(e.entitled) + '</td><td class="num">' + money(e.disbursed) + '</td><td class="num ' + amtClass + '">' + (e.amount < 0 ? '-' : '') + money(e.amount) + '</td><td>' + badge + '</td></tr>';
        });
        html += '</tbody></table>';

        document.getElementById('modalBody').innerHTML = html;
        const canRun = !data.running && !data.balance_error && data.sufficient_funds;
        document.getElementById('confirmBtn').disabled = !canRun;
        document.getElementById('modalFooter').style.display = 'flex';
    }

    function executeDisbursements() {
        const btn = document.getElementById('confirmBtn');
        btn.disabled = true;
        btn.innerHTML = '<span class="spinner"></span> Sending… do not close this tab';
        let url, body;
        if (currentMode === 'custom') {
            url = '/trigger-custom-disbursements';
            body = 'custom_amount=' + encodeURIComponent(currentCustomAmount);
        } else {
            url = '/trigger-disbursements';
            body = '';
        }
        fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body: body })
            .then(r => r.json())
            .then(result => { closeModal(); showResult(result); })
            .catch(err => { closeModal(); showResult({ error: err.message }); });
    }

    function showResult(result) {
        const banner = document.getElementById('resultBanner');
        if (result.error) {
            banner.className = 'result-banner error';
            banner.innerHTML = '<h3>Disbursement run refused or failed</h3><p>' + esc(result.error) + '</p>';
        } else {
            banner.className = 'result-banner ' + (result.failed ? 'error' : 'success');
            banner.innerHTML = '<h3>Run complete</h3><p>Created: ' + result.created + ' · Processed: ' + result.processed + ' · Failed: ' + result.failed + '</p><p>Reloading to show details…</p>';
        }
        setTimeout(() => { location.reload(); }, result.error ? 6000 : 2500);
    }

    function closeModal() {
        document.getElementById('confirmModal').classList.remove('active');
        const btn = document.getElementById('confirmBtn');
        btn.disabled = false;
        btn.innerHTML = 'Confirm &amp; send money';
    }
    </script>
</body>
</html>`
