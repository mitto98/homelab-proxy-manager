<script setup lang="ts">
// Route: /?variant=A|B|C  (stand-in for the Proxy Hosts page)
import { onMounted, onUnmounted, ref } from 'vue'
import VariantA from './VariantA.vue'
import VariantB from './VariantB.vue'
import VariantC from './VariantC.vue'

const variants = [
  { k: 'A', name: 'Sidebar + table + slide-over' },
  { k: 'B', name: 'Top nav + status cards + inline edit' },
  { k: 'C', name: 'Icon rail + master/detail' },
]
const read = () => (new URLSearchParams(location.search).get('variant') ?? 'A').toUpperCase()
const variant = ref(read())
function go(k: string) {
  variant.value = k
  const u = new URL(location.href); u.searchParams.set('variant', k)
  history.replaceState(null, '', u)
}
function cycle(d: number) {
  const i = variants.findIndex(v => v.k === variant.value)
  go(variants[(i + d + variants.length) % variants.length].k)
}
function onKey(e: KeyboardEvent) {
  const t = e.target as HTMLElement
  if (t.closest('input,textarea,select,[contenteditable]')) return
  if (e.key === 'ArrowLeft') cycle(-1)
  if (e.key === 'ArrowRight') cycle(1)
}
onMounted(() => addEventListener('keydown', onKey))
onUnmounted(() => removeEventListener('keydown', onKey))
</script>

<template>
  <VariantA v-if="variant === 'A'" />
  <VariantB v-else-if="variant === 'B'" />
  <VariantC v-else-if="variant === 'C'" />

  <!-- PROTOTYPE switcher: not part of the design being evaluated -->
  <div class="fixed bottom-4 left-1/2 z-[100] flex -translate-x-1/2 items-center gap-3 rounded-full bg-black px-4 py-2 text-sm text-white shadow-xl ring-1 ring-white/20">
    <button class="px-2 text-lg leading-none hover:text-amber-300" @click="cycle(-1)" aria-label="previous">&larr;</button>
    <span class="min-w-64 text-center font-mono">{{ variant }} ({{ variants.find(v => v.k === variant)?.name }})</span>
    <button class="px-2 text-lg leading-none hover:text-amber-300" @click="cycle(1)" aria-label="next">&rarr;</button>
  </div>
</template>
