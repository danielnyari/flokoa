<script setup lang="ts">
import { h, resolveComponent } from 'vue'
import type { TableColumn } from '@nuxt/ui'
import { upperFirst } from 'scule'
import { getPaginationRowModel } from '@tanstack/table-core'
import type { AgentWorkflow } from '~/types'

const UBadge = resolveComponent('UBadge')
const UButton = resolveComponent('UButton')
const UDropdownMenu = resolveComponent('UDropdownMenu')

const toast = useToast()
const router = useRouter()
const table = useTemplateRef('table')

const { namespacedPath, watchUrl: buildWatchUrl } = useFlokoa()

const { items: workflows, status: listStatus, refresh } = useListWatch<AgentWorkflow>({
  listUrl: () => namespacedPath('agentworkflows'),
  watchUrl: () => buildWatchUrl('agentworkflows')
})

const status = computed(() => listStatus.value === 'pending' ? 'pending' : 'success')

const columnFilters = ref([{
  id: 'name',
  value: ''
}])
const columnVisibility = ref()

const readyFilter = ref('all')

watch(() => readyFilter.value, (newVal) => {
  if (!table?.value?.tableApi) return
  const col = table.value.tableApi.getColumn('ready')
  if (!col) return
  if (newVal === 'all') {
    col.setFilterValue(undefined)
  } else {
    col.setFilterValue(newVal === 'true')
  }
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

function readyColor(ready?: boolean): 'success' | 'error' | 'neutral' {
  if (ready === true) return 'success'
  if (ready === false) return 'error'
  return 'neutral'
}

function readyLabel(ready?: boolean): string {
  if (ready === true) return 'Ready'
  if (ready === false) return 'Not Ready'
  return 'Unknown'
}

function getRowItems(row: { original: AgentWorkflow }) {
  return [
    { type: 'label' as const, label: 'Actions' },
    {
      label: 'Copy name',
      icon: 'i-lucide-copy',
      onSelect() {
        navigator.clipboard.writeText(row.original.metadata.name)
        toast.add({ title: 'Copied', description: 'Workflow name copied to clipboard' })
      }
    },
    { type: 'separator' as const },
    {
      label: 'View details',
      icon: 'i-lucide-eye',
      onSelect() {
        router.push(`/workflows/${row.original.metadata.namespace}/${row.original.metadata.name}`)
      }
    }
  ]
}

const columns: TableColumn<AgentWorkflow>[] = [
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
    id: 'description',
    accessorFn: row => row.spec.description ?? '',
    header: 'Description',
    cell: ({ row }) => {
      const desc = row.original.spec.description
      if (!desc) return h('span', { class: 'text-muted' }, '\u2014')
      return h('span', { class: 'text-sm text-muted truncate max-w-xs block' }, desc)
    }
  },
  {
    id: 'ready',
    accessorFn: row => row.status?.ready,
    header: 'Ready',
    filterFn: 'equals',
    cell: ({ row }) => {
      const ready = row.original.status?.ready
      return h(UBadge, { variant: 'subtle', color: readyColor(ready) }, () => readyLabel(ready))
    }
  },
  {
    id: 'tasks',
    accessorFn: row => row.spec.tasks?.length ?? 0,
    header: 'Tasks',
    cell: ({ row }) => `${row.original.spec.tasks?.length ?? 0}`
  },
  {
    id: 'template',
    accessorFn: row => row.status?.workflowTemplateName,
    header: 'Template',
    cell: ({ row }) => {
      const name = row.original.status?.workflowTemplateName
      if (!name) return h('span', { class: 'text-muted' }, '\u2014')
      return h('span', { class: 'text-sm font-mono truncate max-w-48 block' }, name)
    }
  },
  {
    id: 'age',
    accessorFn: row => row.metadata.creationTimestamp,
    header: 'Age',
    cell: ({ row }) => {
      const ts = row.original.metadata.creationTimestamp
      if (!ts) return '\u2014'
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
  <UDashboardPanel id="workflows">
    <template #header>
      <UDashboardNavbar title="Workflows" icon="i-lucide-git-branch">
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
          placeholder="Filter workflows..."
        />

        <div class="flex flex-wrap items-center gap-1.5">
          <USelect
            v-model="readyFilter"
            :items="[
              { label: 'All', value: 'all' },
              { label: 'Ready', value: 'true' },
              { label: 'Not Ready', value: 'false' }
            ]"
            :ui="{ trailingIcon: 'group-data-[state=open]:rotate-180 transition-transform duration-200' }"
            placeholder="Filter status"
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

      <EmptyState
        v-if="workflows.length === 0 && status !== 'pending'"
        icon="i-lucide-git-branch"
        title="No workflows defined"
        description="Create an AgentWorkflow to orchestrate multi-agent tasks with DAG-based execution, conditional branching, and parameter passing."
        docs-url="https://flokoa.ai/getting-started"
        docs-label="Getting Started"
      />

      <template v-else>
        <UTable
          ref="table"
          v-model:column-filters="columnFilters"
          v-model:column-visibility="columnVisibility"
          v-model:pagination="pagination"
          :pagination-options="{ getPaginationRowModel: getPaginationRowModel() }"
          class="shrink-0"
          :data="workflows"
          :columns="columns"
          :loading="status === 'pending'"
          :ui="{
            base: 'table-fixed border-separate border-spacing-0',
            thead: '[&>tr]:bg-elevated/50 [&>tr]:after:content-none',
            tbody: '[&>tr]:last:[&>td]:border-b-0 [&>tr]:cursor-pointer [&>tr]:hover:bg-elevated/50',
            th: 'py-2 first:rounded-l-lg last:rounded-r-lg border-y border-default first:border-l last:border-r',
            td: 'border-b border-default',
            separator: 'h-0'
          }"
          @select="(_e: Event, row: { original: AgentWorkflow }) => router.push(`/workflows/${row.original.metadata.namespace}/${row.original.metadata.name}`)"
        />

        <div class="flex items-center justify-between gap-3 border-t border-default pt-4 mt-auto">
          <div class="text-sm text-muted">
            {{ table?.tableApi?.getFilteredRowModel().rows.length || 0 }} workflow(s)
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
    </template>
  </UDashboardPanel>
</template>
