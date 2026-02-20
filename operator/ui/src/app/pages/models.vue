<script setup lang="ts">
import { h, resolveComponent } from 'vue'
import type { TableColumn } from '@nuxt/ui'
import { upperFirst } from 'scule'
import { getPaginationRowModel } from '@tanstack/table-core'
import type { Model } from '~/types'

const UBadge = resolveComponent('UBadge')
const UButton = resolveComponent('UButton')
const UDropdownMenu = resolveComponent('UDropdownMenu')

const toast = useToast()
const table = useTemplateRef('table')

const selectedModel = ref<Model | null>(null)
const detailOpen = ref(false)

function openDetail(model: Model) {
  selectedModel.value = model
  detailOpen.value = true
}

const { listModels } = useFlokoa()
const { data: modelList, status, refresh } = await listModels()

const models = computed(() => modelList.value?.items ?? [])

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

function getRowItems(row: { original: Model }) {
  return [
    { type: 'label' as const, label: 'Actions' },
    {
      label: 'Copy name',
      icon: 'i-lucide-copy',
      onSelect() {
        navigator.clipboard.writeText(row.original.metadata.name)
        toast.add({ title: 'Copied', description: 'Model name copied to clipboard' })
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

const columns: TableColumn<Model>[] = [
  {
    id: 'name',
    accessorFn: row => row.metadata.name,
    header: 'Name',
    cell: ({ row }) => {
      return h('div', undefined, [
        h('p', { class: 'font-medium text-highlighted' }, row.original.metadata.name),
        h('p', { class: 'text-sm text-muted' }, row.original.metadata.namespace)
      ])
    }
  },
  {
    id: 'model',
    accessorFn: row => row.spec.model,
    header: 'Model ID',
    cell: ({ row }) => {
      return h('span', { class: 'font-mono text-sm' }, row.original.spec.model)
    }
  },
  {
    id: 'provider',
    header: 'Provider',
    cell: ({ row }) => {
      const resolved = row.original.status?.resolvedProvider
      const providerName = row.original.spec.providerRef.name
      if (resolved?.provider) {
        return h('div', { class: 'flex items-center gap-2' }, [
          h(UBadge, { variant: 'subtle', color: 'neutral' }, () => resolved.provider),
          h('span', { class: 'text-sm text-muted' }, providerName)
        ])
      }
      return h('span', { class: 'text-sm' }, providerName)
    }
  },
  {
    id: 'temperature',
    header: 'Temperature',
    cell: ({ row }) => row.original.spec.parameters?.temperature ?? '—'
  },
  {
    id: 'maxTokens',
    header: 'Max Tokens',
    cell: ({ row }) => {
      const val = row.original.spec.parameters?.maxTokens
      return val ? val.toLocaleString() : '—'
    }
  },
  {
    id: 'ready',
    header: 'Ready',
    cell: ({ row }) => {
      const ready = row.original.status?.ready
      if (ready === undefined) return h('span', { class: 'text-muted' }, '—')
      const color = ready ? 'success' as const : 'error' as const
      return h(UBadge, { variant: 'subtle', color }, () => ready ? 'Ready' : 'Not Ready')
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
  <UDashboardPanel id="models">
    <template #header>
      <UDashboardNavbar title="Models" icon="i-lucide-brain">
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
          placeholder="Filter models..."
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
        :data="models"
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
          {{ table?.tableApi?.getFilteredRowModel().rows.length || 0 }} model(s)
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

  <ModelDetail v-if="selectedModel" v-model:open="detailOpen" :model="selectedModel" />
</template>
