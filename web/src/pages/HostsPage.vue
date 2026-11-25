<script setup lang="ts">
import { ref } from 'vue'
import type { Host } from '@/api/client'
import HostSheet from '@/components/hosts/HostSheet.vue'
import HostsTable from '@/components/hosts/HostsTable.vue'
import { useHostsQuery } from '@/hosts/queries'

const { data: hosts, isPending, error } = useHostsQuery()

// null means "adding a new host". The host is kept while the sheet closes so its
// content does not change during the exit animation.
const open = ref(false)
const editing = ref<Host | null>(null)

function openSheet(host: Host | null) {
  editing.value = host
  open.value = true
}
</script>

<template>
  <!-- Plain loading and error text; the polished states are slice [6.4]. -->
  <p v-if="isPending" class="text-muted-foreground text-sm">Loading proxy hosts...</p>
  <p v-else-if="error" role="alert" class="text-destructive text-sm">
    Could not load the proxy hosts: {{ error.message }}
  </p>
  <HostsTable v-else :hosts="hosts ?? []" @add="openSheet(null)" @edit="openSheet" />
  <HostSheet :open="open" :host="editing" @close="open = false" />
</template>
