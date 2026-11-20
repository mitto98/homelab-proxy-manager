<script setup lang="ts">
// Variant C: master/detail. Narrow icon rail, compact searchable list on the left,
// the selected host's form permanently open on the right with a sticky save bar.
// Form is a single compact page grouped in cards; a live "what Traefik will do" summary sits beside it.
import { computed, reactive, ref, watch } from 'vue'
import { hosts, blank, save, remove, certOptions, accessLists, statusClass, dark, toggleDark, certLabel, type Host } from './data'

const rail = [['⇄', 'Proxy Hosts'], ['↪', 'Redirection Hosts'], ['🔒', 'Certificates'], ['🛡', 'Access Lists'], ['👤', 'Users'], ['⚙', 'Settings']]
const q = ref('')
const list = computed(() => hosts.filter(h => h.domains.join(' ').toLowerCase().includes(q.value.toLowerCase())))
const selectedId = ref<number>(hosts[0].id)
const draft = ref<Host>(reactive(JSON.parse(JSON.stringify(hosts[0]))))
const domainsText = ref(hosts[0].domains.join('\n'))
function select(id: number) {
  selectedId.value = id
  const h = id === 0 ? blank() : hosts.find(x => x.id === id)!
  draft.value = reactive(JSON.parse(JSON.stringify(h)))
  domainsText.value = draft.value.domains.join('\n')
}
watch(domainsText, v => { draft.value.domains = v.split('\n').map(s => s.trim()).filter(Boolean) })
function submit() { save(draft.value); select(draft.value.id || hosts[hosts.length - 1].id) }
const dirty = computed(() => selectedId.value === 0 || JSON.stringify(draft.value) !== JSON.stringify(hosts.find(h => h.id === selectedId.value)))
const summary = computed(() => {
  const d = draft.value, s: string[] = []
  s.push(`${d.domains.join(', ') || '(no domain)'} → ${d.scheme}://${d.upstream || '?'}:${d.port}`)
  if (d.cert !== 'none') s.push(`TLS via ${certLabel(d.cert)}`)
  if (d.forceHttps) s.push('HTTP requests redirect to HTTPS')
  if (d.hsts) s.push(`HSTS header${d.hstsSub ? ' with subdomains' : ''}`)
  if (d.accessList !== 'none') s.push(`Restricted by "${d.accessList}"`)
  if (!d.enabled) s.push('Disabled: no routers generated')
  return s
})
const inp = 'w-full rounded border border-zinc-300 bg-white px-2.5 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900'
const dot = { active: 'bg-emerald-500', failed: 'bg-red-500', disabled: 'bg-zinc-400' }
</script>

<template>
  <div class="flex h-screen overflow-hidden">
    <aside class="flex w-14 shrink-0 flex-col items-center gap-1 bg-zinc-900 py-3 text-zinc-300">
      <div class="mb-3 text-xl">◈</div>
      <a v-for="(r, i) in rail" :key="r[1]" href="#" :title="r[1]" class="flex h-10 w-10 items-center justify-center rounded-lg text-lg hover:bg-zinc-700" :class="i === 0 ? 'bg-zinc-700 text-white' : ''">{{ r[0] }}</a>
      <button class="mt-auto h-10 w-10 rounded-lg hover:bg-zinc-700" @click="toggleDark">{{ dark ? '☀' : '☾' }}</button>
    </aside>

    <section class="flex w-80 shrink-0 flex-col border-r border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
      <div class="space-y-2 border-b border-zinc-200 p-3 dark:border-zinc-800">
        <div class="flex items-center justify-between"><h1 class="font-semibold">Proxy Hosts</h1><button class="rounded bg-zinc-900 px-2.5 py-1 text-sm text-white dark:bg-zinc-100 dark:text-zinc-900" @click="select(0)">+ New</button></div>
        <input v-model="q" :class="inp" placeholder="Filter…" />
      </div>
      <ul class="flex-1 overflow-y-auto">
        <li v-if="selectedId === 0" class="border-b border-zinc-100 bg-indigo-50 px-3 py-2.5 text-sm italic dark:border-zinc-800 dark:bg-indigo-950">New host (unsaved)</li>
        <li v-for="h in list" :key="h.id" class="cursor-pointer border-b border-zinc-100 px-3 py-2.5 dark:border-zinc-800" :class="h.id === selectedId ? 'bg-indigo-50 dark:bg-indigo-950' : 'hover:bg-zinc-50 dark:hover:bg-zinc-800'" @click="select(h.id)">
          <div class="flex items-center gap-2"><span class="h-2 w-2 rounded-full" :class="dot[h.status]"></span><span class="truncate text-sm font-medium">{{ h.domains[0] }}</span></div>
          <div class="ml-4 truncate font-mono text-xs text-zinc-500">{{ h.upstream }}:{{ h.port }}</div>
        </li>
      </ul>
    </section>

    <form class="flex flex-1 flex-col overflow-hidden" @submit.prevent="submit">
      <div class="flex-1 overflow-y-auto p-8">
        <div class="mx-auto grid max-w-4xl grid-cols-[1fr_16rem] gap-8">
          <div class="space-y-5">
            <div class="flex items-center gap-3"><h2 class="text-xl font-semibold">{{ draft.id ? draft.domains[0] : 'New Proxy Host' }}</h2><span v-if="draft.id" class="rounded-full px-2 py-0.5 text-xs" :class="statusClass[draft.status]">{{ draft.status }}</span></div>
            <p v-if="draft.error" class="rounded border border-red-300 bg-red-50 p-3 font-mono text-xs text-red-800 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{{ draft.error }}</p>

            <div class="rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
              <label class="mb-1 block text-sm font-medium">Domains <span class="font-normal text-zinc-500">one per line, *.example.com for wildcards</span></label>
              <textarea v-model="domainsText" rows="3" :class="inp + ' font-mono'"></textarea>
              <label class="mb-1 mt-3 block text-sm font-medium">Forward to</label>
              <div class="flex gap-2"><select v-model="draft.scheme" :class="inp + ' !w-24'"><option>http</option><option>https</option></select><input v-model="draft.upstream" :class="inp" placeholder="host or IP" /><input v-model.number="draft.port" type="number" :class="inp + ' !w-24'" /></div>
              <label v-if="draft.scheme === 'https'" class="mt-2 flex items-center gap-2 text-sm"><input v-model="draft.skipTlsVerify" type="checkbox" /> Skip upstream TLS verification</label>
            </div>

            <div class="grid grid-cols-2 gap-5">
              <div class="space-y-2 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
                <div class="text-sm font-medium">Certificate</div>
                <select v-model="draft.cert" :class="inp"><option v-for="c in certOptions" :key="c.v" :value="c.v">{{ c.label }}</option></select>
                <label class="flex items-center gap-2 text-sm"><input v-model="draft.forceHttps" type="checkbox" /> Force HTTPS</label>
                <label class="flex items-center gap-2 text-sm"><input v-model="draft.hsts" type="checkbox" /> HSTS</label>
                <label v-if="draft.hsts" class="ml-6 flex items-center gap-2 text-sm"><input v-model="draft.hstsSub" type="checkbox" /> Include subdomains</label>
              </div>
              <div class="space-y-2 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
                <div class="text-sm font-medium">Access</div>
                <select v-model="draft.accessList" :class="inp"><option v-for="a in accessLists" :key="a" :value="a">{{ a === 'none' ? 'Public' : a }}</option></select>
                <label class="flex items-center gap-2 text-sm"><input v-model="draft.enabled" type="checkbox" /> Enabled</label>
              </div>
            </div>
          </div>

          <aside class="h-fit rounded-lg border border-dashed border-zinc-300 p-4 text-sm dark:border-zinc-700">
            <div class="mb-2 text-xs font-semibold uppercase text-zinc-500">What will happen</div>
            <ul class="space-y-2"><li v-for="s in summary" :key="s">{{ s }}</li></ul>
          </aside>
        </div>
      </div>
      <div class="flex items-center justify-between border-t border-zinc-200 bg-white px-8 py-3 dark:border-zinc-800 dark:bg-zinc-900">
        <button v-if="draft.id" type="button" class="text-sm text-red-600" @click="remove(draft.id); select(hosts[0].id)">Delete host</button><span v-else></span>
        <div class="flex items-center gap-3"><span v-if="dirty" class="text-xs text-amber-600">Unsaved changes</span><button class="rounded bg-indigo-600 px-4 py-2 text-sm text-white disabled:opacity-40" :disabled="!dirty">Save &amp; apply</button></div>
      </div>
    </form>
  </div>
</template>
