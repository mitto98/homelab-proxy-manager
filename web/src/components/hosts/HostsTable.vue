<script setup lang="ts">
import {
  createColumnHelper,
  FlexRender,
  columnFilteringFeature,
  globalFilteringFeature,
  createFilteredRowModel,
  tableFeatures,
  useTable,
} from '@tanstack/vue-table'
import { computed, h, ref } from 'vue'
import type { Host } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const props = defineProps<{ hosts: Host[] }>()
const emit = defineEmits<{ add: []; edit: [host: Host] }>()

// Only what the table uses is registered (TanStack Table v9 tree-shakes the rest).
const features = tableFeatures({
  columnFilteringFeature,
  globalFilteringFeature,
  filteredRowModel: createFilteredRowModel(),
})
const columnHelper = createColumnHelper<typeof features, Host>()

const columns = columnHelper.columns([
  // The search box filters on this column's text: every domain, not just the first.
  columnHelper.accessor((host) => host.domains.join(' '), {
    id: 'domains',
    header: 'Domains',
    cell: ({ row }) => {
      const [first, ...more] = row.original.domains
      return h('div', { class: 'flex items-center gap-2' }, [
        h('span', { class: 'font-medium' }, first),
        more.length > 0
          ? h(
              Badge,
              { variant: 'secondary', title: row.original.domains.join('\n') },
              () => `+${more.length}`,
            )
          : null,
      ])
    },
  }),
  columnHelper.display({
    id: 'upstream',
    header: 'Upstream',
    cell: ({ row }) => {
      const h0 = row.original
      return h(
        'span',
        { class: 'font-mono text-xs' },
        `${h0.upstreamScheme}://${h0.upstreamHost}:${h0.upstreamPort}`,
      )
    },
  }),
  columnHelper.display({
    id: 'status',
    header: 'Status',
    cell: ({ row }) =>
      row.original.enabled
        ? h(
            Badge,
            {
              variant: 'outline',
              class:
                'border-emerald-600/40 bg-emerald-600/10 text-emerald-700 dark:text-emerald-400',
            },
            () => 'active',
          )
        : h(Badge, { variant: 'secondary' }, () => 'disabled'),
  }),
  columnHelper.display({
    id: 'actions',
    header: () => h('span', { class: 'sr-only' }, 'Actions'),
    cell: ({ row }) =>
      h(
        Button,
        {
          variant: 'ghost',
          size: 'sm',
          'aria-label': `Edit ${row.original.domains[0]}`,
          onClick: () => emit('edit', row.original),
        },
        () => 'Edit',
      ),
  }),
])

const search = ref('')
const table = useTable({
  features,
  columns,
  get data() {
    return props.hosts
  },
  state: {
    get globalFilter() {
      return search.value.trim()
    },
  },
})

const rows = computed(() => table.getRowModel().rows)
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-3">
      <Input
        v-model="search"
        type="search"
        placeholder="Search domains"
        aria-label="Search domains"
        class="max-w-xs"
      />
      <Button @click="emit('add')">Add Proxy Host</Button>
    </div>
    <div class="overflow-hidden rounded-lg border">
      <Table>
        <TableHeader class="bg-muted/50">
          <TableRow v-for="group in table.getHeaderGroups()" :key="group.id">
            <TableHead v-for="header in group.headers" :key="header.id">
              <FlexRender v-if="!header.isPlaceholder" :header="header" />
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in rows" :key="row.id" :data-testid="`host-row-${row.original.id}`">
            <TableCell v-for="cell in row.getAllCells()" :key="cell.id">
              <FlexRender :cell="cell" />
            </TableCell>
          </TableRow>
          <TableRow v-if="rows.length === 0">
            <TableCell :colspan="columns.length" class="text-muted-foreground h-24 text-center">
              {{ hosts.length === 0 ? 'No proxy hosts yet' : `No hosts match "${search.trim()}"` }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>
