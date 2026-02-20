<script setup lang="ts">
import { h, resolveComponent } from 'vue'
import type { TableColumn } from '@nuxt/ui'
import { upperFirst } from 'scule'
import { getPaginationRowModel } from '@tanstack/table-core'
import type { AgentTool } from '~/types'

const UBadge = resolveComponent('UBadge')
const UButton = resolveComponent('UButton')
const UDropdownMenu = resolveComponent('UDropdownMenu')

const toast = useToast()
const table = useTemplateRef('table')

const selectedTool = ref<AgentTool | null>(null)
const detailOpen = ref(false)

function openDetail(tool: AgentTool) {
  selectedTool.value = tool
  detailOpen.value = true
}

const { listAgentTools } = useFlokoa()
const { data: toolList, status, refresh } = await listAgentTools()

const tools = computed(() => toolList.value?.items ?? [])

const columnFilters = ref([{
  id: 'name',
  value: ''
}])
const columnVisibility = ref()

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

function getToolSource(tool: AgentTool): string {
  if (tool.spec.openApi?.url) return tool.spec.openApi.url
  if (tool.spec.openApi?.serviceRef) {
    const ref = tool.spec.openApi.serviceRef
    const port = ref.port ?? ref.portName ?? ''
    return `${ref.name}${port ? `:${port}` : ''}`
  }
  return '—'
}

function getRowItems(row: { original: AgentTool }) {
  return [
    { type: 'label' as const, label: 'Actions' },
    {
      label: 'Copy name',
      icon: 'i-lucide-copy',
      onSelect() {
        navigator.clipboard.writeText(row.original.metadata.name)
        toast.add({ title: 'Copied', description: 'Tool name copied to clipboard' })
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

const columns: TableColumn<AgentTool>[] = [
  {
    id: 'name',
    accessorFn: (row) => row.metadata.name,
    header: 'Name',
    cell: ({ row }) => {
      return h('div', undefined, [
        h('p', { class: 'font-medium text-highlighted' }, row.original.metadata.name),
        h('p', { class: 'text-sm text-muted' }, row.original.metadata.namespace)
      ])
    }
  },
  {
    id: 'type',
    accessorFn: (row) => row.spec.type,
    header: 'Type',
    cell: ({ row }) => {
      return h(UBadge, { variant: 'outline', color: 'neutral', class: 'uppercase text-xs' }, () => row.original.spec.type)
    }
  },
  {
    id: 'description',
    accessorFn: (row) => row.spec.description,
    header: 'Description',
    cell: ({ row }) => {
      const desc = row.original.spec.description
      const truncated = desc.length > 80 ? desc.slice(0, 80) + '...' : desc
      return h('span', { class: 'text-sm', title: desc }, truncated)
    }
  },
  {
    id: 'source',
    header: 'Source',
    cell: ({ row }) => {
      const source = getToolSource(row.original)
      const isUrl = source.startsWith('http')
      if (isUrl) return h('span', { class: 'text-sm font-mono truncate max-w-48 block' }, source)
      if (source === '—') return h('span', { class: 'text-muted' }, '—')
      return h('div', { class: 'flex items-center gap-1' }, [
        h(resolveComponent('UIcon'), { name: 'i-lucide-server', class: 'size-3.5 text-muted' }),
        h('span', { class: 'text-sm font-mono' }, source)
      ])
    }
  },
  {
    id: 'timeout',
    header: 'Timeout',
    cell: ({ row }) => {
      const timeout = row.original.spec.openApi?.timeoutSeconds ?? 30
      return `${timeout}s`
    }
  },
  {
    id: 'age',
    accessorFn: (row) => row.metadata.creationTimestamp,
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
  <UDashboardPanel id="tools">
    <template #header>
      <UDashboardNavbar title="Agent Tools" icon="i-lucide-wrench">
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
          placeholder="Filter tools..."
        />

        <div class="flex flex-wrap items-center gap-1.5">
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
        :data="tools"
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
          {{ table?.tableApi?.getFilteredRowModel().rows.length || 0 }} tool(s)
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

  <ToolDetail v-if="selectedTool" v-model:open="detailOpen" :tool="selectedTool" />
</template>
