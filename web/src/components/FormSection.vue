<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useId } from 'vue'
import { cn } from '@/lib/utils'

// One distinct block of a long form: its own heading, a short description and a
// visible border. The Proxy Host slide-over stacks these (Details now, SSL in
// [3.2], Access in [5.3]) so the form never reads as one continuous scroll.
defineProps<{ title: string; description?: string; class?: HTMLAttributes['class'] }>()
const headingId = useId()
</script>

<template>
  <section
    :aria-labelledby="headingId"
    :class="
      cn('bg-card text-card-foreground space-y-4 rounded-lg border p-5 shadow-xs', $props.class)
    "
  >
    <header class="space-y-1">
      <h3 :id="headingId" class="text-sm font-semibold">{{ title }}</h3>
      <p v-if="description" class="text-muted-foreground text-sm">{{ description }}</p>
    </header>
    <slot />
  </section>
</template>
