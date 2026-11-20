<script setup lang="ts">
// Variant A: classic admin. Left sidebar, dense data table, form in a right slide-over (sheet),
// one scrolling form with section headings.
import { computed, ref } from 'vue'
import { hosts, blank, save, remove, certOptions, accessLists, statusClass, certLabel, dark, toggleDark, type Host } from './data'

const nav = ['Proxy Hosts', 'Redirection Hosts', 'Certificates', 'Access Lists', 'Users', 'Settings']
const q = ref('')
const rows = computed(() => hosts.filter(h => h.domains.join(' ').toLowerCase().includes(q.value.toLowerCase())))
const editing = ref<Host | null>(null)
const domainInput = ref('')
const open = (h?: Host) => { editing.value = h ? JSON.parse(JSON.stringify(h)) : blank(); domainInput.value = '' }
function addDomain() {
  const d = domainInput.value.trim()
  if (d && editing.value && !editing.value.domains.includes(d)) editing.value.domains.push(d)
  domainInput.value = ''
}
function submit() { if (editing.value) { save(editing.value); editing.value = null } }
const inp = 'w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900'
</script>

<template>
  <div class="flex min-h-screen">
    <aside class="flex w-60 shrink-0 flex-col border-r border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
      <div class="px-5 py-4 text-lg font-semibold">Proxy Manager</div>
      <nav class="flex-1 space-y-1 px-3">
        <a v-for="n in nav" :key="n" href="#" class="block rounded-md px-3 py-2 text-sm"
           :class="n === 'Proxy Hosts' ? 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900' : 'text-zinc-600 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800'">{{ n }}</a>
      </nav>
      <div class="flex items-center justify-between border-t border-zinc-200 px-4 py-3 text-sm dark:border-zinc-800">
        <span>admin</span>
        <button class="rounded border px-2 py-1 text-xs dark:border-zinc-700" @click="toggleDark">{{ dark ? 'Light' : 'Dark' }}</button>
      </div>
    </aside>

    <main class="flex-1 p-8">
      <div class="mb-6 flex items-center justify-between">
        <h1 class="text-2xl font-semibold">Proxy Hosts</h1>
        <div class="flex gap-3">
          <input v-model="q" placeholder="Search domains" :class="inp + ' w-64'" />
          <button class="rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white dark:bg-zinc-100 dark:text-zinc-900" @click="open()">Add Proxy Host</button>
        </div>
      </div>

      <div class="overflow-hidden rounded-lg border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
        <table class="w-full text-left text-sm">
          <thead class="border-b border-zinc-200 bg-zinc-50 text-xs uppercase text-zinc-500 dark:border-zinc-800 dark:bg-zinc-950">
            <tr><th class="px-4 py-3">Domains</th><th class="px-4 py-3">Upstream</th><th class="px-4 py-3">Certificate</th><th class="px-4 py-3">Access</th><th class="px-4 py-3">Status</th><th class="px-4 py-3"></th></tr>
          </thead>
          <tbody>
            <tr v-for="h in rows" :key="h.id" class="border-b border-zinc-100 last:border-0 dark:border-zinc-800">
              <td class="px-4 py-3 font-medium">{{ h.domains[0] }}<span v-if="h.domains.length > 1" class="ml-2 rounded bg-zinc-100 px-1.5 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400">+{{ h.domains.length - 1 }}</span></td>
              <td class="px-4 py-3 font-mono text-xs">{{ h.scheme }}://{{ h.upstream }}:{{ h.port }}</td>
              <td class="px-4 py-3">{{ certLabel(h.cert) }}</td>
              <td class="px-4 py-3">{{ h.accessList === 'none' ? 'Public' : h.accessList }}</td>
              <td class="px-4 py-3"><span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="statusClass[h.status]" :title="h.error">{{ h.status }}</span></td>
              <td class="px-4 py-3 text-right"><button class="text-sm underline" @click="open(h)">Edit</button></td>
            </tr>
          </tbody>
        </table>
      </div>
    </main>

    <!-- slide-over sheet -->
    <div v-if="editing" class="fixed inset-0 z-50 flex justify-end bg-black/40" @click.self="editing = null">
      <form class="flex h-full w-[28rem] flex-col bg-white shadow-2xl dark:bg-zinc-900" @submit.prevent="submit">
        <div class="border-b border-zinc-200 px-6 py-4 text-lg font-semibold dark:border-zinc-800">{{ editing.id ? 'Edit' : 'Add' }} Proxy Host</div>
        <div class="flex-1 space-y-6 overflow-y-auto px-6 py-5">
          <section class="space-y-3">
            <h3 class="text-xs font-semibold uppercase text-zinc-500">Details</h3>
            <div>
              <label class="mb-1 block text-sm">Domain names</label>
              <div class="mb-2 flex flex-wrap gap-1"><span v-for="d in editing.domains" :key="d" class="rounded bg-zinc-100 px-2 py-0.5 text-sm dark:bg-zinc-800">{{ d }} <button type="button" @click="editing.domains = editing.domains.filter(x => x !== d)">&times;</button></span></div>
              <input v-model="domainInput" :class="inp" placeholder="example.com, press Enter" @keydown.enter.prevent="addDomain" />
            </div>
            <div class="grid grid-cols-[6rem_1fr_6rem] gap-2">
              <select v-model="editing.scheme" :class="inp"><option>http</option><option>https</option></select>
              <input v-model="editing.upstream" :class="inp" placeholder="192.168.1.10" />
              <input v-model.number="editing.port" type="number" :class="inp" />
            </div>
            <label v-if="editing.scheme === 'https'" class="flex items-center gap-2 text-sm"><input v-model="editing.skipTlsVerify" type="checkbox" /> Skip upstream TLS verification</label>
          </section>
          <section class="space-y-3">
            <h3 class="text-xs font-semibold uppercase text-zinc-500">SSL</h3>
            <select v-model="editing.cert" :class="inp"><option v-for="c in certOptions" :key="c.v" :value="c.v">{{ c.label }}</option></select>
            <label class="flex items-center gap-2 text-sm"><input v-model="editing.forceHttps" type="checkbox" /> Force HTTPS</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="editing.hsts" type="checkbox" /> HSTS</label>
            <label v-if="editing.hsts" class="ml-6 flex items-center gap-2 text-sm"><input v-model="editing.hstsSub" type="checkbox" /> Include subdomains</label>
          </section>
          <section class="space-y-3">
            <h3 class="text-xs font-semibold uppercase text-zinc-500">Access</h3>
            <select v-model="editing.accessList" :class="inp"><option v-for="a in accessLists" :key="a" :value="a">{{ a === 'none' ? 'Publicly accessible' : a }}</option></select>
            <label class="flex items-center gap-2 text-sm"><input v-model="editing.enabled" type="checkbox" /> Enabled</label>
          </section>
        </div>
        <div class="flex justify-between border-t border-zinc-200 px-6 py-4 dark:border-zinc-800">
          <button v-if="editing.id" type="button" class="text-sm text-red-600" @click="remove(editing.id); editing = null">Delete</button><span v-else></span>
          <div class="flex gap-2">
            <button type="button" class="rounded-md border px-4 py-2 text-sm dark:border-zinc-700" @click="editing = null">Cancel</button>
            <button class="rounded-md bg-zinc-900 px-4 py-2 text-sm text-white dark:bg-zinc-100 dark:text-zinc-900">Save</button>
          </div>
        </div>
      </form>
    </div>
  </div>
</template>
