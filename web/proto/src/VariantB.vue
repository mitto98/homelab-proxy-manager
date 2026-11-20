<script setup lang="ts">
// Variant B: status-first. Top nav, health summary strip, one card per host.
// Failed hosts surface their Traefik error on the card. Edit expands the card in place,
// with the form split into tabs (Upstream / TLS / Access).
import { computed, reactive, ref } from 'vue'
import { hosts, blank, save, remove, certOptions, accessLists, statusClass, dark, toggleDark, type Host } from './data'

const nav = ['Proxy Hosts', 'Redirects', 'Certificates', 'Access Lists', 'Users', 'Settings']
const counts = computed(() => ({
  active: hosts.filter(h => h.status === 'active').length,
  failed: hosts.filter(h => h.status === 'failed').length,
  disabled: hosts.filter(h => h.status === 'disabled').length,
}))
const filter = ref<'all' | 'active' | 'failed' | 'disabled'>('all')
const shown = computed(() => hosts.filter(h => filter.value === 'all' || h.status === filter.value))
const draft = ref<Host | null>(null)
const cards = computed(() => (draft.value && draft.value.id === 0 ? [draft.value, ...shown.value] : shown.value))
const tab = ref<'Upstream' | 'TLS' | 'Access'>('Upstream')
const domainsText = ref('')
const open = (h?: Host) => { draft.value = reactive(h ? JSON.parse(JSON.stringify(h)) : blank()); domainsText.value = draft.value!.domains.join(', '); tab.value = 'Upstream' }
function submit() {
  if (!draft.value) return
  draft.value.domains = domainsText.value.split(',').map(s => s.trim()).filter(Boolean)
  save(draft.value); draft.value = null
}
const inp = 'w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-950'
</script>

<template>
  <div class="min-h-screen">
    <header class="border-b border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
      <div class="mx-auto flex max-w-5xl items-center gap-8 px-6 py-3">
        <span class="font-semibold">Proxy Manager</span>
        <nav class="flex flex-1 gap-5 text-sm">
          <a v-for="n in nav" :key="n" href="#" :class="n === 'Proxy Hosts' ? 'border-b-2 border-indigo-500 pb-1 font-medium' : 'text-zinc-500'">{{ n }}</a>
        </nav>
        <button class="text-sm text-zinc-500" @click="toggleDark">{{ dark ? '☀' : '☾' }}</button>
        <span class="text-sm text-zinc-500">admin</span>
      </div>
    </header>

    <main class="mx-auto max-w-5xl px-6 py-8">
      <div class="mb-5 grid grid-cols-3 gap-4">
        <button v-for="(n, k) in counts" :key="k" class="rounded-xl border p-4 text-left transition"
                :class="[filter === k ? 'border-indigo-500 ring-2 ring-indigo-500/30' : 'border-zinc-200 dark:border-zinc-800', 'bg-white dark:bg-zinc-900']" @click="filter = filter === k ? 'all' : k">
          <div class="text-3xl font-semibold" :class="k === 'failed' && n ? 'text-red-600' : ''">{{ n }}</div>
          <div class="text-sm capitalize text-zinc-500">{{ k }}</div>
        </button>
      </div>

      <div class="mb-3 flex items-center justify-between">
        <h1 class="text-lg font-semibold">{{ filter === 'all' ? 'All hosts' : filter + ' hosts' }}</h1>
        <button class="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white" @click="open()">+ New host</button>
      </div>

      <div class="space-y-3">
        <div v-for="h in cards" :key="h.id" class="rounded-xl border bg-white dark:bg-zinc-900"
             :class="h.status === 'failed' ? 'border-red-300 dark:border-red-800' : 'border-zinc-200 dark:border-zinc-800'">
          <div class="flex items-center gap-4 px-5 py-4">
            <span class="h-2.5 w-2.5 rounded-full" :class="{ 'bg-emerald-500': h.status === 'active', 'bg-red-500': h.status === 'failed', 'bg-zinc-400': h.status === 'disabled' }"></span>
            <div class="flex-1">
              <div class="font-medium">{{ h.id ? h.domains.join(', ') : 'New Proxy Host' }}</div>
              <div class="font-mono text-xs text-zinc-500">&rarr; {{ h.scheme }}://{{ h.upstream }}:{{ h.port }}</div>
            </div>
            <span v-if="h.cert !== 'none'" class="rounded-full bg-indigo-50 px-2 py-0.5 text-xs text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300">🔒 TLS</span>
            <span v-if="h.accessList !== 'none'" class="rounded-full bg-amber-50 px-2 py-0.5 text-xs text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">{{ h.accessList }}</span>
            <span class="rounded-full px-2 py-0.5 text-xs" :class="statusClass[h.status]">{{ h.status }}</span>
            <button class="text-sm text-indigo-600" @click="draft?.id === h.id ? draft = null : open(h)">{{ draft?.id === h.id ? 'Close' : 'Edit' }}</button>
          </div>
          <div v-if="h.error" class="border-t border-red-200 bg-red-50 px-5 py-2 font-mono text-xs text-red-800 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{{ h.error }}</div>

          <!-- inline editor -->
          <form v-if="draft && draft.id === h.id" class="border-t border-zinc-200 px-5 py-4 dark:border-zinc-800" @submit.prevent="submit">
            <div class="mb-4 flex gap-1 border-b border-zinc-200 dark:border-zinc-800">
              <button v-for="t in (['Upstream', 'TLS', 'Access'] as const)" :key="t" type="button" class="px-3 py-2 text-sm" :class="tab === t ? 'border-b-2 border-indigo-500 font-medium' : 'text-zinc-500'" @click="tab = t">{{ t }}</button>
            </div>
            <div v-show="tab === 'Upstream'" class="space-y-3">
              <input v-model="domainsText" :class="inp" placeholder="Domains, comma separated" />
              <div class="flex gap-2"><select v-model="draft.scheme" :class="inp + ' !w-28'"><option>http</option><option>https</option></select><input v-model="draft.upstream" :class="inp" /><input v-model.number="draft.port" type="number" :class="inp + ' !w-28'" /></div>
              <label v-if="draft.scheme === 'https'" class="flex items-center gap-2 text-sm"><input v-model="draft.skipTlsVerify" type="checkbox" /> Skip TLS verification</label>
            </div>
            <div v-show="tab === 'TLS'" class="space-y-3">
              <select v-model="draft.cert" :class="inp"><option v-for="c in certOptions" :key="c.v" :value="c.v">{{ c.label }}</option></select>
              <label class="flex items-center gap-2 text-sm"><input v-model="draft.forceHttps" type="checkbox" /> Force HTTPS</label>
              <label class="flex items-center gap-2 text-sm"><input v-model="draft.hsts" type="checkbox" /> HSTS <span v-if="draft.hsts" class="ml-3"><input v-model="draft.hstsSub" type="checkbox" /> include subdomains</span></label>
            </div>
            <div v-show="tab === 'Access'" class="space-y-3">
              <select v-model="draft.accessList" :class="inp"><option v-for="a in accessLists" :key="a" :value="a">{{ a === 'none' ? 'Public' : a }}</option></select>
              <label class="flex items-center gap-2 text-sm"><input v-model="draft.enabled" type="checkbox" /> Enabled</label>
            </div>
            <div class="mt-4 flex justify-between">
              <button v-if="h.id" type="button" class="text-sm text-red-600" @click="remove(h.id); draft = null">Delete</button><span v-else></span>
              <button class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white">Save changes</button>
            </div>
          </form>
        </div>
      </div>
    </main>
  </div>
</template>
