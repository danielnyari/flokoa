<script setup lang="ts">
import { h, resolveComponent } from 'vue'
import type { TableColumn } from '@nuxt/ui'
import { upperFirst } from 'scule'
import { getPaginationRowModel } from '@tanstack/table-core'
import type { Agent } from '~/types'

const UBadge = resolveComponent('UBadge')
const UButton = resolveComponent('UButton')
const UDropdownMenu = resolveComponent('UDropdownMenu')

const toast = useToast()
const table = useTemplateRef('table')

const selectedAgent = ref<Agent | null>(null)
const detailOpen = ref(false)

function openDetail(agent: Agent) {
  selectedAgent.value = agent
  detailOpen.value = true
}

const { namespacedPath, watchUrl: buildWatchUrl } = useFlokoa()

const { items: agents, status: listStatus, refresh } = useListWatch<Agent>({
  listUrl: () => namespacedPath('agents'),
  watchUrl: () => buildWatchUrl('agents')
})

const status = computed(() => listStatus.value === 'pending' ? 'pending' : 'success')

const columnFilters = ref([{
  id: 'name',
  value: ''
}])
const columnVisibility = ref()

const phaseFilter = ref('all')

watch(() => phaseFilter.value, (newVal) => {
  if (!table?.value?.tableApi) return
  const col = table.value.tableApi.getColumn('phase')
  if (!col) return
  col.setFilterValue(newVal === 'all' ? undefined : newVal)
})

const nameSearch = computed({
  get: (): string => {
    return (table.value?.tableApi?.getColumn('name')?.getFilterValue() as string) || ''
  },
  set: (value: string) => {
    table.value?.tableApi?.getColumn('name')?.setFilterValue(value || undefined)
  }
})

const pagination = ref({
  pageIndex: 0,
  pageSize: 10
})

function getRowItems(row: { original: Agent }) {
  return [
    { type: 'label' as const, label: 'Actions' },
    {
      label: 'Copy name',
      icon: 'i-lucide-copy',
      onSelect() {
        navigator.clipboard.writeText(row.original.metadata.name)
        toast.add({ title: 'Copied', description: 'Agent name copied to clipboard' })
      }
    },
    { type: 'separator' as const },
    {
      label: 'View details',
      icon: 'i-lucide-eye',
      onSelect() {
        openDetail(row.original)
      }
    }
  ]
}

const columns: TableColumn<Agent>[] = [
  {
    id: 'name',
    accessorFn: row => row.metadata.name,
    header: 'Name',
    cell: ({ row }) => {
      return h('div', { class: 'flex items-center gap-2' }, [
        h('div', undefined, [
          h('p', { class: 'font-medium text-highlighted' }, row.original.metadata.name),
          h('p', { class: 'text-sm text-muted' }, row.original.metadata.namespace)
        ])
      ])
    }
  },
  {
    id: 'phase',
    accessorFn: row => row.status?.phase,
    header: 'Phase',
    filterFn: 'equals',
    cell: ({ row }) => {
      const phase = row.original.status?.phase ?? 'Unknown'
      const color = {
        Running: 'success' as const,
        Pending: 'warning' as const,
        Failed: 'error' as const,
        Unknown: 'neutral' as const
      }[phase] ?? 'neutral' as const
      return h(UBadge, { class: 'capitalize', variant: 'subtle', color }, () => phase)
    }
  },
  {
    id: 'framework',
    accessorFn: row => row.spec.framework ?? row.status?.detectedFramework,
    header: 'Framework',
    cell: ({ row }) => {
      const fw = row.original.spec.framework ?? row.original.status?.detectedFramework
      if (!fw) return h('span', { class: 'text-muted' }, '—')
      return h(UBadge, { variant: 'outline', color: 'neutral' }, () => fw)
    }
  },
  {
    id: 'runtime',
    accessorFn: row => row.spec.runtime?.type,
    header: 'Runtime',
    cell: ({ row }) => row.original.spec.runtime?.type ?? '—'
  },
  {
    id: 'replicas',
    header: 'Ready',
    cell: ({ row }) => {
      const avail = row.original.status?.availableReplicas ?? 0
      const total = row.original.status?.replicas ?? row.original.spec.runtime?.standard?.replicas ?? 0
      return `${avail}/${total}`
    }
  },
  {
    id: 'url',
    accessorFn: row => row.status?.url,
    header: 'URL',
    cell: ({ row }) => {
      const url = row.original.status?.url
      if (!url) return h('span', { class: 'text-muted' }, '—')
      return h('span', { class: 'text-sm font-mono truncate max-w-48 block' }, url)
    }
  },
  {
    id: 'age',
    accessorFn: row => row.metadata.creationTimestamp,
    header: 'Age',
    cell: ({ row }) => {
      const ts = row.original.metadata.creationTimestamp
      if (!ts) return '—'
      return useTimeAgo(ts).value
    }
  },
  {
    id: 'actions',
    cell: ({ row }) => {
      return h(
        'div',
        { class: 'text-right' },
        h(
          UDropdownMenu,
          { content: { align: 'end' }, items: getRowItems(row) },
          () => h(UButton, { icon: 'i-lucide-ellipsis-vertical', color: 'neutral', variant: 'ghost', class: 'ml-auto' })
        )
      )
    }
  }
]
</script>

<template>
  <UDashboardPanel id="agents">
    <template #header>
      <UDashboardNavbar title="Agents" icon="i-lucide-bot">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
        <template #trailing>
          <UButton
            icon="i-lucide-refresh-cw"
            color="neutral"
            variant="ghost"
            :loading="status === 'pending'"
            @click="refresh()"
          />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div class="flex flex-wrap items-center justify-between gap-1.5">
        <UInput
          v-model="nameSearch"
          class="max-w-sm"
          icon="i-lucide-search"
          placeholder="Filter agents..."
        />

        <div class="flex flex-wrap items-center gap-1.5">
          <USelect
            v-model="phaseFilter"
            :items="[
              { label: 'All', value: 'all' },
              { label: 'Running', value: 'Running' },
              { label: 'Pending', value: 'Pending' },
              { label: 'Failed', value: 'Failed' }
            ]"
            :ui="{ trailingIcon: 'group-data-[state=open]:rotate-180 transition-transform duration-200' }"
            placeholder="Filter phase"
            class="min-w-28"
          />
          <UDropdownMenu
            :items="
              table?.tableApi
                ?.getAllColumns()
                .filter((column: any) => column.getCanHide())
                .map((column: any) => ({
                  label: upperFirst(column.id),
                  type: 'checkbox' as const,
                  checked: column.getIsVisible(),
                  onUpdateChecked(checked: boolean) {
                    table?.tableApi?.getColumn(column.id)?.toggleVisibility(!!checked)
                  },
                  onSelect(e?: Event) {
                    e?.preventDefault()
                  }
                }))
            "
            :content="{ align: 'end' }"
          >
            <UButton
              label="Display"
              color="neutral"
              variant="outline"
              trailing-icon="i-lucide-settings-2"
            />
          </UDropdownMenu>
        </div>
      </div>

      <UTable
        ref="table"
        v-model:column-filters="columnFilters"
        v-model:column-visibility="columnVisibility"
        v-model:pagination="pagination"
        :pagination-options="{ getPaginationRowModel: getPaginationRowModel() }"
        class="shrink-0"
        :data="agents"
        :columns="columns"
        :loading="status === 'pending'"
        :ui="{
          base: 'table-fixed border-separate border-spacing-0',
          thead: '[&>tr]:bg-elevated/50 [&>tr]:after:content-none',
          tbody: '[&>tr]:last:[&>td]:border-b-0',
          th: 'py-2 first:rounded-l-lg last:rounded-r-lg border-y border-default first:border-l last:border-r',
          td: 'border-b border-default',
          separator: 'h-0'
        }"
      />

      <div class="flex items-center justify-between gap-3 border-t border-default pt-4 mt-auto">
        <div class="text-sm text-muted">
          {{ table?.tableApi?.getFilteredRowModel().rows.length || 0 }} agent(s)
        </div>

        <div class="flex items-center gap-1.5">
          <UPagination
            :default-page="(table?.tableApi?.getState().pagination.pageIndex || 0) + 1"
            :items-per-page="table?.tableApi?.getState().pagination.pageSize"
            :total="table?.tableApi?.getFilteredRowModel().rows.length"
            @update:page="(p: number) => table?.tableApi?.setPageIndex(p - 1)"
          />
        </div>
      </div>
    </template>
  </UDashboardPanel>

  <AgentDetail v-if="selectedAgent" v-model:open="detailOpen" :agent="selectedAgent" />
</template>
