<script setup lang="ts">
import HostsTable from '@/components/hosts/HostsTable.vue'
import { useHostsQuery } from '@/hosts/queries'

const { data: hosts, isPending, error } = useHostsQuery()
</script>

<template>
  <!-- Plain loading and error text; the polished states are slice [6.4]. -->
  <p v-if="isPending" class="text-muted-foreground text-sm">Loading proxy hosts...</p>
  <p v-else-if="error" role="alert" class="text-destructive text-sm">
    Could not load the proxy hosts: {{ error.message }}
  </p>
  <HostsTable v-else :hosts="hosts ?? []" />
</template>
