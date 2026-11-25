import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import HostsTable from './HostsTable.vue'
import type { Host } from '@/api/client'
import { host } from '@/test/hosts'

const hosts = [
  host({ id: 1 }),
  host({
    id: 2,
    domains: ['nas.example.com', 'files.example.com', 'dav.example.com'],
    upstreamScheme: 'https',
    upstreamHost: 'nas',
    upstreamPort: 5001,
    enabled: false,
  }),
]

const mountTable = (list: Host[] = hosts) => mount(HostsTable, { props: { hosts: list } })
const rowTexts = (w: ReturnType<typeof mountTable>) =>
  w.findAll('tbody tr').map((r) => r.text().replace(/\s+/g, ' '))

describe('HostsTable', () => {
  it('has the Domains, Upstream and Status columns and an Edit action', () => {
    const w = mountTable()
    expect(w.findAll('thead th').map((h) => h.text())).toEqual([
      'Domains',
      'Upstream',
      'Status',
      'Actions',
    ])
    expect(w.findAll('tbody tr')).toHaveLength(2)
    expect(w.findAll('button').filter((b) => b.text() === 'Edit')).toHaveLength(2)
  })

  it('shows the first domain with a +n badge listing all domains', () => {
    const w = mountTable()
    const [single, multi] = w.findAll('tbody tr')
    expect(single!.text()).toContain('app.example.com')
    expect(single!.find('[data-slot=badge][title]').exists()).toBe(false)
    expect(multi!.text()).toContain('nas.example.com')
    expect(multi!.text()).not.toContain('files.example.com')
    const badge = multi!.get('[data-slot=badge][title]')
    expect(badge.text()).toBe('+2')
    expect(badge.attributes('title')).toBe('nas.example.com\nfiles.example.com\ndav.example.com')
  })

  it('shows the upstream as scheme://host:port in monospace', () => {
    const cell = mountTable().findAll('tbody tr')[1]!.findAll('td')[1]!
    expect(cell.text()).toBe('https://nas:5001')
    expect(cell.get('span').classes()).toContain('font-mono')
  })

  it('derives the status from enabled: only active or disabled', () => {
    const rows = mountTable().findAll('tbody tr')
    expect(rows[0]!.findAll('td')[2]!.text()).toBe('active')
    expect(rows[1]!.findAll('td')[2]!.text()).toBe('disabled')
  })

  it('emits edit with the host and add from the buttons', async () => {
    const w = mountTable()
    await w
      .findAll('button')
      .find((b) => b.text() === 'Edit')!
      .trigger('click')
    expect(w.emitted('edit')?.[0]).toEqual([hosts[0]])
    await w
      .findAll('button')
      .find((b) => b.text() === 'Add Proxy Host')!
      .trigger('click')
    expect(w.emitted('add')).toHaveLength(1)
  })

  it('filters by any domain, case-insensitively, and restores on clearing', async () => {
    const w = mountTable()
    const search = w.get('input[type=search]')
    await search.setValue('FILES.exa')
    expect(rowTexts(w)).toHaveLength(1)
    expect(rowTexts(w)[0]).toContain('nas.example.com')
    await search.setValue('  app  ')
    expect(rowTexts(w)).toHaveLength(1)
    expect(rowTexts(w)[0]).toContain('app.example.com')
    await search.setValue('')
    expect(rowTexts(w)).toHaveLength(2)
  })

  it('says so when the search matches nothing', async () => {
    const w = mountTable()
    await w.get('input[type=search]').setValue('nope')
    expect(w.text()).toContain('No hosts match "nope"')
  })

  it('shows a plain empty state with no hosts', () => {
    const w = mountTable([])
    expect(w.text()).toContain('No proxy hosts yet')
    expect(w.text()).toContain('Add Proxy Host')
  })
})
