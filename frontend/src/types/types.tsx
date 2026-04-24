type UploadState = 'idle' | 'uploading' | 'processing' | 'success' | 'error'

type JobStatus = {
  id: string
  filename: string
  status: string
  result?: Record<string, unknown>
}

type Toast = {
  id: number
  kind: 'success' | 'error'
  message: string
}


type NormalizedResult = {
    category?: string
    newName?: string
    originalName?: string
  }
  

export type { UploadState, JobStatus, Toast, NormalizedResult }