import { useCallback, useEffect, useRef, useState } from 'react'
import { UploadState, JobStatus, Toast } from './types/types'

import { formatBytes, iconFor, iconForCategory, stepIndexFor, normalizeResult } from './utils'

// Steps reflect the job lifecycle:
//   uploading (browser→backend)  →  PENDING (queued)  →  TRIGGERED (n8n running)  →  DONE
const STEPS = [
  { key: 'upload', label: 'Upload' },
  { key: 'queued', label: 'Queued' },
  { key: 'triggered', label: 'Processing' },
  { key: 'done', label: 'Done' },
] as const

function App() {
  const [file, setFile] = useState<File | null>(null)
  const [state, setState] = useState<UploadState>('idle')
  const [progress, setProgress] = useState(0)
  const [dragActive, setDragActive] = useState(false)
  const [toasts, setToasts] = useState<Toast[]>([])
  const [jobId, setJobId] = useState<string | null>(null)
  const [job, setJob] = useState<JobStatus | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const pushToast = useCallback((kind: Toast['kind'], message: string) => {
    const id = Date.now() + Math.random()
    setToasts((t) => [...t, { id, kind, message }])
    setTimeout(() => {
      setToasts((t) => t.filter((x) => x.id !== id))
    }, 4000)
  }, [])

  const onSelect = (f: File | null) => {
    if (!f) return
    setFile(f)
    setState('idle')
    setProgress(0)
  }

  const onDrop = (e: React.DragEvent<HTMLLabelElement>) => {
    e.preventDefault()
    setDragActive(false)
    const f = e.dataTransfer.files?.[0]
    if (f) onSelect(f)
  }

  const reset = () => {
    setFile(null)
    setState('idle')
    setProgress(0)
    setJobId(null)
    setJob(null)
    if (inputRef.current) inputRef.current.value = ''
  }

  // Subscribe to real-time job events over WebSocket. The backend also pushes
  // a "snapshot" message on connect, so we don't miss terminal events that
  // happened before the socket was ready.
  useEffect(() => {
    if (!jobId) return

    let cancelled = false
    let ws: WebSocket | null = null
    let reconnectTimer: number | undefined
    let attempts = 0

    const apply = (data: JobStatus) => {
      if (cancelled) return
      setJob((prev) => ({
        id: data.id ?? prev?.id ?? jobId,
        filename: data.filename ?? prev?.filename ?? '',
        status: data.status,
        result: data.result ?? prev?.result,
      }))

      if (data.status === 'DONE') {
        setState('success')
        pushToast('success', `Job "${data.filename ?? jobId}" finished`)
        cancelled = true
        ws?.close()
      } else if (data.status === 'FAILED') {
        setState('error')
        pushToast('error', `Job "${data.filename ?? jobId}" failed`)
        cancelled = true
        ws?.close()
      }
    }

    const connect = () => {
      if (cancelled) return
      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url = `${proto}//${window.location.host}/api/ws?jobId=${encodeURIComponent(jobId)}`
      ws = new WebSocket(url)

      ws.onopen = () => {
        attempts = 0
      }

      ws.onmessage = (ev) => {
        try {
          const msg = JSON.parse(ev.data) as JobStatus & { type?: string }
          apply(msg)
        } catch {
          // ignore malformed frames
        }
      }

      ws.onerror = () => {
        // onclose will fire right after; handle reconnection there.
      }

      ws.onclose = () => {
        if (cancelled) return
        attempts++
        const delay = Math.min(10000, 500 * 2 ** Math.min(attempts, 5))
        reconnectTimer = window.setTimeout(connect, delay)
      }
    }

    // Safety net: fetch once immediately in case the job already reached a
    // terminal state before the socket connects (e.g. very fast workflows).
    fetch(`/api/job/status?id=${encodeURIComponent(jobId)}`)
      .then((r) => (r.ok ? r.json() : null))
      .then((data: JobStatus | null) => {
        if (data) apply(data)
      })
      .catch(() => {})

    connect()

    return () => {
      cancelled = true
      if (reconnectTimer) window.clearTimeout(reconnectTimer)
      ws?.close()
    }
  }, [jobId, pushToast])

  const upload = () => {
    if (!file) return
    setState('uploading')
    setProgress(0)

    const form = new FormData()
    form.append('file', file)

    const xhr = new XMLHttpRequest()
    xhr.open('POST', '/api/upload')

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) {
        setProgress(Math.round((e.loaded / e.total) * 100))
      }
    }

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        setProgress(100)
        try {
          const res = JSON.parse(xhr.responseText)
          if (res.jobId) {
            setJobId(res.jobId)
            setState('processing')
            pushToast('success', `Uploaded "${res.filename}" — processing…`)
          } else {
            setState('success')
            pushToast('success', 'Upload complete')
          }
        } catch {
          setState('success')
          pushToast('success', 'Upload complete')
        }
      } else {
        setState('error')
        pushToast('error', `Upload failed (${xhr.status})`)
      }
    }

    xhr.onerror = () => {
      setState('error')
      pushToast('error', 'Network error during upload')
    }

    xhr.send(form)
  }

  return (
    <div className="page">
      <header className="hero">
        <h1>📦 File Automation</h1>
        <p>Drop a file, we'll store it and kick off the n8n workflow.</p>
      </header>

      <main className="card">
        <label
          htmlFor="file-input"
          className={`dropzone ${dragActive ? 'drag' : ''} ${file ? 'has-file' : ''}`}
          onDragOver={(e) => {
            e.preventDefault()
            setDragActive(true)
          }}
          onDragLeave={() => setDragActive(false)}
          onDrop={onDrop}
        >
          {!file ? (
            <>
              <div className="dz-icon">⬆️</div>
              <div className="dz-title">Drag &amp; drop a file here</div>
              <div className="dz-sub">or click to browse</div>
            </>
          ) : (
            <div className="preview">
              <div className="preview-icon">{iconFor(file.name)}</div>
              <div className="preview-meta">
                <div className="preview-name" title={file.name}>{file.name}</div>
                <div className="preview-size">{formatBytes(file.size)}</div>
              </div>
              <button
                className="icon-btn"
                onClick={(e) => {
                  e.preventDefault()
                  reset()
                }}
                aria-label="Remove file"
                disabled={state === 'uploading' || state === 'processing'}
              >
                ✕
              </button>
            </div>
          )}
          <input
            id="file-input"
            ref={inputRef}
            type="file"
            onChange={(e) => onSelect(e.target.files?.[0] ?? null)}
            hidden
          />
        </label>

        {state === 'uploading' && (
          <div className="progress" aria-label="Upload progress">
            <div className="progress-bar" style={{ width: `${progress}%` }} />
            <span className="progress-label">{progress}%</span>
          </div>
        )}

        <div className="actions">
          <button
            className="primary"
            onClick={state === 'success' ? reset : upload}
            disabled={state === 'success' ? false : !file || state === 'uploading' || state === 'processing'}
          >
            {state === 'uploading'
              ? 'Uploading…'
              : state === 'processing'
                ? 'Processing…'
                : state === 'success'
                  ? 'Upload another'
                  : 'Upload & process'}
          </button>
          {file && state !== 'uploading' && state !== 'processing' && state !== 'success' && (
            <button className="ghost" onClick={reset}>Clear</button>
          )}
        </div>

        {(state === 'uploading' || state === 'processing' || state === 'success' || state === 'error') && (
          <ol className={`stepper ${state === 'error' ? 'has-error' : ''}`} aria-label="Job progress">
            {STEPS.map((step, idx) => {
              const current = stepIndexFor(state, job?.status)
              const done = idx < current || (idx === current && state === 'success')
              const active = idx === current && state !== 'success' && state !== 'error'
              const errored = state === 'error' && idx === current
              return (
                <li
                  key={step.key}
                  className={`step ${done ? 'done' : ''} ${active ? 'active' : ''} ${errored ? 'errored' : ''}`}
                >
                  <span className="step-dot" aria-hidden="true">
                    {done ? '✓' : errored ? '!' : idx + 1}
                  </span>
                  <span className="step-label">{step.label}</span>
                </li>
              )
            })}
          </ol>
        )}

        {state === 'success' && (() => {
          const r = normalizeResult(job?.result)
          if (!r) {
            return <div className="status ok">✅ Done — no result payload returned.</div>
          }
          return (
            <div className="result-card" role="status">
              <div className="result-head">
                <div className="result-badge">
                  <span className="result-badge-icon">{r.category ? iconForCategory(r.category) : '📁'}</span>
                  <span className="result-badge-text">
                    <span className="result-badge-label">Category</span>
                    <span className="result-badge-value">{r.category ?? 'Uncategorized'}</span>
                  </span>
                </div>
                <span className="result-check" aria-hidden="true">✓</span>
              </div>

              <dl className="result-details">
                {r.originalName && (
                  <>
                    <dt>Original</dt>
                    <dd title={r.originalName}>{r.originalName}</dd>
                  </>
                )}
                {r.newName && (
                  <>
                    <dt>Saved as</dt>
                    <dd className="mono" title={r.newName}>{r.newName}</dd>
                  </>
                )}
              </dl>
            </div>
          )
        })()}

        {state === 'error' && (
          <div className="status err">⚠️ Something went wrong. Check the backend logs.</div>
        )}
      </main>

      <div className="toasts" role="status" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={`toast ${t.kind}`}>{t.message}</div>
        ))}
      </div>
    </div>
  )
}

export default App
