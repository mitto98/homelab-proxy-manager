import { reactive, ref } from 'vue'

export type Status = 'active' | 'failed' | 'disabled'
export interface Host {
  id: number
  domains: string[]
  scheme: 'http' | 'https'
  upstream: string
  port: number
  skipTlsVerify: boolean
  cert: string // 'none' | 'acme:<name>' | 'custom:<name>'
  accessList: string // 'none' | name
  forceHttps: boolean
  hsts: boolean
  hstsSub: boolean
  enabled: boolean
  status: Status
  error?: string
}

export const certOptions = [
  { v: 'none', label: 'None (HTTP only)' },
  { v: 'acme:letsencrypt-http', label: 'Let’s Encrypt (HTTP challenge)' },
  { v: 'acme:letsencrypt-dns', label: 'Let’s Encrypt (DNS, wildcard ok)' },
  { v: 'custom:internal-ca', label: 'Custom: internal-ca (expires 2027-03-01)' },
]
export const accessLists = ['none', 'Family only', 'Admins (basic auth)', 'LAN 192.168.0.0/16']

export const hosts = reactive<Host[]>([
  { id: 1, domains: ['photos.home.example'], scheme: 'http', upstream: '192.168.1.20', port: 2283, skipTlsVerify: false, cert: 'acme:letsencrypt-http', accessList: 'Family only', forceHttps: true, hsts: true, hstsSub: false, enabled: true, status: 'active' },
  { id: 2, domains: ['*.lab.example', 'lab.example'], scheme: 'https', upstream: 'proxmox.lan', port: 8006, skipTlsVerify: true, cert: 'acme:letsencrypt-dns', accessList: 'LAN 192.168.0.0/16', forceHttps: true, hsts: false, hstsSub: false, enabled: true, status: 'active' },
  { id: 3, domains: ['git.home.example'], scheme: 'http', upstream: '192.168.1.31', port: 3000, skipTlsVerify: false, cert: 'acme:letsencrypt-http', accessList: 'none', forceHttps: true, hsts: true, hstsSub: true, enabled: true, status: 'failed', error: 'service “git@file”: no healthy servers (dial tcp 192.168.1.31:3000: connection refused)' },
  { id: 4, domains: ['grafana.home.example', 'metrics.home.example'], scheme: 'http', upstream: '192.168.1.40', port: 3001, skipTlsVerify: false, cert: 'custom:internal-ca', accessList: 'Admins (basic auth)', forceHttps: true, hsts: false, hstsSub: false, enabled: true, status: 'active' },
  { id: 5, domains: ['old-wiki.home.example'], scheme: 'http', upstream: '192.168.1.50', port: 8080, skipTlsVerify: false, cert: 'none', accessList: 'none', forceHttps: false, hsts: false, hstsSub: false, enabled: false, status: 'disabled' },
  { id: 6, domains: ['ha.home.example'], scheme: 'http', upstream: '192.168.1.60', port: 8123, skipTlsVerify: false, cert: 'acme:letsencrypt-http', accessList: 'none', forceHttps: true, hsts: true, hstsSub: false, enabled: true, status: 'active' },
])

export const blank = (): Host => ({
  id: 0, domains: [], scheme: 'http', upstream: '', port: 80, skipTlsVerify: false, cert: 'none',
  accessList: 'none', forceHttps: false, hsts: false, hstsSub: false, enabled: true, status: 'active',
})

// stub "save": mutates in memory only
export function save(h: Host) {
  if (h.id === 0) hosts.push({ ...h, id: Math.max(...hosts.map(x => x.id)) + 1, status: h.enabled ? 'active' : 'disabled' })
  else Object.assign(hosts.find(x => x.id === h.id)!, h, { status: h.enabled ? (h.status === 'disabled' ? 'active' : h.status) : 'disabled' })
}
export function remove(id: number) { hosts.splice(hosts.findIndex(x => x.id === id), 1) }

export const dark = ref(false)
export function toggleDark() {
  dark.value = !dark.value
  document.documentElement.classList.toggle('dark', dark.value)
}

export const statusClass: Record<Status, string> = {
  active: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300',
  failed: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  disabled: 'bg-zinc-200 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400',
}
export const certLabel = (v: string) => certOptions.find(c => c.v === v)?.label.split(' (')[0] ?? v
