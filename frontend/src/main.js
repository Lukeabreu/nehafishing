// Wails injects window.go bindings and window.runtime at startup.
// All public methods on App in app.go become: window.go.main.App.MethodName()
// Events arrive via window.runtime.EventsOn()

const { PickFolder, SetFolder, GetArcheAgeStatus, OpenExplorer } = window.go.main.App
const { EventsOn } = window.runtime

const btnPick      = document.getElementById('btn-pick')
const btnExplorer  = document.getElementById('btn-explorer')
const btnClear     = document.getElementById('btn-clear')
const statusBar    = document.getElementById('status-bar')
const logBox       = document.getElementById('log-box')
const processBadge = document.getElementById('process-badge')
const folderCard   = document.getElementById('folder-card')
const folderPath   = document.getElementById('folder-path')
const checkAction  = document.getElementById('check-action')
const checkLog     = document.getElementById('check-log')

// ── Helpers ───────────────────────────────────────────────────────────────

function appendLog(text, cls) {
    const line = document.createElement('div')
    line.className = 'line-' + (cls || 'go')

    const ts = new Date().toLocaleTimeString('en-GB', { hour12: false })
    line.textContent = `[${ts}] ${text}`

    logBox.appendChild(line)
    logBox.scrollTop = logBox.scrollHeight

    while (logBox.children.length > 500) {
        logBox.removeChild(logBox.firstChild)
    }
}

function setStatus(text) {
    statusBar.textContent = text
}

/**
 * Apply a FolderStatus object (returned from Go's SetFolder / PickFolder)
 * to update the UI: status bar, folder card, file check indicators.
 */
function applyFolderStatus(fs) {
    if (!fs || !fs.path) {
        // Cancelled or empty
        return
    }

    setStatus(fs.status_msg)

    // Show folder card
    folderCard.classList.remove('hidden')
    folderPath.textContent = fs.path

    // File existence indicators
    checkAction.textContent = (fs.has_action_file ? '✅' : '❌') + ' neha-fishing-actions.txt'
    checkAction.className   = 'file-check ' + (fs.has_action_file ? 'file-ok' : 'file-missing')

    checkLog.textContent = (fs.has_log_file ? '✅' : '❌') + ' neha-fishing-log.txt'
    checkLog.className   = 'file-check ' + (fs.has_log_file ? 'file-ok' : 'file-missing')

    appendLog('Folder set: ' + fs.path)
}

// ── Pick folder (native dialog) ───────────────────────────────────────────

btnPick.addEventListener('click', async () => {
    btnPick.disabled = true
    btnPick.textContent = '…'
    try {
        const fs = await PickFolder()
        applyFolderStatus(fs)
    } catch (e) {
        setStatus('Error: ' + e)
        appendLog('Error: ' + e, 'err')
    } finally {
        btnPick.disabled = false
        btnPick.textContent = '📂 Choose Folder…'
    }
})

// ── Open Explorer ─────────────────────────────────────────────────────────

btnExplorer.addEventListener('click', () => {
    OpenExplorer()
})

// ── Clear log ─────────────────────────────────────────────────────────────

btnClear.addEventListener('click', () => {
    logBox.innerHTML = ''
    appendLog('Log cleared.')
})

// ── Events from Go ────────────────────────────────────────────────────────

// Fish detected + R sent
EventsOn('scan:result', msg => {
    appendLog(msg, msg.startsWith('🐟') ? 'fish' : 'go')
})

// Addon log lines tailed from neha-fishing-log.txt
EventsOn('log:lines', lines => {
    for (const line of lines) {
        appendLog(line, 'addon')
    }
})

// ArcheAge process status
EventsOn('process:status', running => {
    if (running) {
        processBadge.textContent = 'ArcheAge ●'
        processBadge.className = 'badge badge-on'
    } else {
        processBadge.textContent = 'ArcheAge ○'
        processBadge.className = 'badge badge-off'
    }
})

// ── Initial process status ────────────────────────────────────────────────

GetArcheAgeStatus().then(running => {
    processBadge.textContent = running ? 'ArcheAge ●' : 'ArcheAge ○'
    processBadge.className   = running ? 'badge badge-on' : 'badge badge-off'
})

appendLog('NehaFishing ready — click "Choose Folder" to select your addon directory.')