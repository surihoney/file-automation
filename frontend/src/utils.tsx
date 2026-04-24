import { UploadState, NormalizedResult } from './types/types'

export function formatBytes(bytes: number): string {
    if (bytes === 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB']
    const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)))
    return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function iconFor(name: string): string {
    const ext = name.split('.').pop()?.toLowerCase() ?? ''
    if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg'].includes(ext)) return '🖼️'
    if (['pdf'].includes(ext)) return '📄'
    if (['doc', 'docx', 'txt', 'md'].includes(ext)) return '📝'
    if (['xls', 'xlsx', 'csv'].includes(ext)) return '📊'
    if (['zip', 'tar', 'gz', 'rar'].includes(ext)) return '🗜️'
    if (['mp3', 'wav', 'flac'].includes(ext)) return '🎵'
    if (['mp4', 'mov', 'avi', 'mkv'].includes(ext)) return '🎬'
    return '📁'
}

export function iconForCategory(category: string): string {
    const c = category.toLowerCase()
    if (c === 'photos') return '🖼️'
    if (c === 'documents') return '📄'
    if (c === 'videos') return '🎬'
    if (c === 'audio' || c === 'music') return '🎵'
    if (c === 'archives') return '🗜️'
    return '📁'
}

export function stepIndexFor(state: UploadState, status?: string): number {
    if (state === 'idle' || state === 'uploading') return 0
    switch (status) {
      case 'PENDING':
        return 1
      case 'TRIGGERED':
        return 2
      case 'DONE':
        return 3
      default:
        return state === 'success' ? 3 : 1
    }
  }
  
  // Tolerate both shapes we've seen in the wild:
  //   { category, newName }                                    (n8n-reshaped)
  //   { data: { category, new_name, original_name } }          (raw file-processor)
  export function normalizeResult(raw: Record<string, unknown> | undefined): NormalizedResult | null {
    if (!raw) return null
    const nested = (raw.data ?? raw.result) as Record<string, unknown> | undefined
    const merged: Record<string, unknown> = { ...(nested ?? {}), ...raw }
  
    const pick = (...keys: string[]): string | undefined => {
      for (const k of keys) {
        const v = merged[k]
        if (typeof v === 'string' && v.length > 0) return v
      }
      return undefined
    }
  
    const res: NormalizedResult = {
      category: pick('category', 'Category'),
      newName: pick('newName', 'new_name', 'newPath', 'new_path'),
      originalName: pick('originalName', 'original_name'),
    }
    if (!res.category && !res.newName && !res.originalName) return null
    return res
  }