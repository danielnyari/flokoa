<script setup lang="ts">
import { h, resolveComponent } from 'vue'
import type { TableColumn } from '@nuxt/ui'
import { upperFirst } from 'scule'
import { getPaginationRowModel } from '@tanstack/table-core'
import type { AgentWorkflow, WorkflowPhase } from '~/types'

const UBadge = resolveComponent('UBadge')
const UButton = resolveComponent('UButton')
const UDropdownMenu = resolveComponent('UDropdownMenu')

const toast = useToast()
const router = useRouter()
const table = useTemplateRef('table')

const { listAgentWorkflows } = useFlokoa()
const { data: workflowList, status, refresh } = await listAgentWorkflows()

const workflows = computed(() => workflowList.value?.items ?? [])

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

function phaseColor(phase?: WorkflowPhase): 'success' | 'error' | 'warning' | 'neutral' | 'info' {
  switch (phase) {
    case 'WORKFLOW_PHASE_READY': return 'success'
    case 'WORKFLOW_PHASE_COMPILING': return 'warning'
    case 'WORKFLOW_PHASE_ERROR': return 'error'
    case 'WORKFLOW_PHASE_PENDING': return 'neutral'
    default: return 'neutral'
  }
}

function phaseLabel(phase?: WorkflowPhase): string {
  switch (phase) {
    case 'WORKFLOW_PHASE_READY': return 'Ready'
    case 'WORKFLOW_PHASE_COMPILING': return 'Compiling'
    case 'WORKFLOW_PHASE_ERROR': return 'Error'
    case 'WORKFLOW_PHASE_PENDING': return 'Pending'
    default: return 'Unknown'
  }
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
    id: 'phase',
    accessorFn: row => row.status?.phase,
    header: 'Phase',
    filterFn: 'equals',
    cell: ({ row }) => {
      const phase = row.original.status?.phase
      return h(UBadge, { variant: 'subtle', color: phaseColor(phase) }, () => phaseLabel(phase))
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
            v-model="phaseFilter"
            :items="[
              { label: 'All', value: 'all' },
              { label: 'Ready', value: 'WORKFLOW_PHASE_READY' },
              { label: 'Compiling', value: 'WORKFLOW_PHASE_COMPILING' },
              { label: 'Error', value: 'WORKFLOW_PHASE_ERROR' },
              { label: 'Pending', value: 'WORKFLOW_PHASE_PENDING' }
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
        :data="workflows"
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
  </UDashboardPanel>
</template>
