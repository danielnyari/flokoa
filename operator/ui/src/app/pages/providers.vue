<script setup lang="ts">
import { h, resolveComponent } from 'vue'
import type { TableColumn } from '@nuxt/ui'
import { upperFirst } from 'scule'
import { getPaginationRowModel } from '@tanstack/table-core'
import type { ModelProvider } from '~/types'

const UBadge = resolveComponent('UBadge')
const UButton = resolveComponent('UButton')
const UDropdownMenu = resolveComponent('UDropdownMenu')

const toast = useToast()
const table = useTemplateRef('table')

const selectedProvider = ref<ModelProvider | null>(null)
const detailOpen = ref(false)

function openDetail(provider: ModelProvider) {
  selectedProvider.value = provider
  detailOpen.value = true
}

const { namespacedPath, watchUrl: buildWatchUrl } = useFlokoa()

const { items: providers, status: listStatus, refresh } = useListWatch<ModelProvider>({
  listUrl: () => namespacedPath('modelproviders'),
  watchUrl: () => buildWatchUrl('modelproviders')
})

const status = computed(() => listStatus.value === 'pending' ? 'pending' : 'success')

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

const providerTypeFilter = ref('all')

watch(() => providerTypeFilter.value, (newVal) => {
  if (!table?.value?.tableApi) return
  const col = table.value.tableApi.getColumn('providerType')
  if (!col) return
  col.setFilterValue(newVal === 'all' ? undefined : newVal)
})

const pagination = ref({
  pageIndex: 0,
  pageSize: 10
})

const providerTypeMap: Record<string | number, string> = {
  PROVIDER_TYPE_OPENAI: 'openai',
  PROVIDER_TYPE_ANTHROPIC: 'anthropic',
  PROVIDER_TYPE_GOOGLE: 'google',
  PROVIDER_TYPE_BEDROCK: 'bedrock',
  1: 'openai',
  2: 'anthropic',
  3: 'google',
  4: 'bedrock'
}

function getProviderType(provider: ModelProvider): string {
  const raw = provider.status?.provider
  if (raw && providerTypeMap[raw]) return providerTypeMap[raw]
  if (provider.spec.openai) return 'openai'
  if (provider.spec.anthropic) return 'anthropic'
  if (provider.spec.google) return 'google'
  if (provider.spec.bedrock) return 'bedrock'
  return 'unknown'
}

const providerLabel: Record<string, string> = {
  openai: 'OpenAI',
  anthropic: 'Anthropic',
  google: 'Google',
  bedrock: 'Bedrock'
}

function getProviderIcon(type: string): string {
  const icons: Record<string, string> = {
    openai: 'i-simple-icons-openai',
    anthropic: 'i-simple-icons-anthropic',
    google: 'i-simple-icons-google',
    bedrock: 'i-simple-icons-amazonaws'
  }
  return icons[type] ?? 'i-lucide-cloud'
}

function getRowItems(row: { original: ModelProvider }) {
  return [
    { type: 'label' as const, label: 'Actions' },
    {
      label: 'Copy name',
      icon: 'i-lucide-copy',
      onSelect() {
        navigator.clipboard.writeText(row.original.metadata.name)
        toast.add({ title: 'Copied', description: 'Provider name copied to clipboard' })
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

const columns: TableColumn<ModelProvider>[] = [
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
    id: 'providerType',
    accessorFn: row => getProviderType(row),
    header: 'Provider',
    filterFn: 'equals',
    cell: ({ row }) => {
      const type = getProviderType(row.original)
      return h('div', { class: 'flex items-center gap-2' }, [
        h(resolveComponent('UIcon'), { name: getProviderIcon(type), class: 'size-4' }),
        h(UBadge, { variant: 'subtle', color: 'neutral' }, () => providerLabel[type] ?? type)
      ])
    }
  },
  {
    id: 'endpoint',
    header: 'Endpoint',
    cell: ({ row }) => {
      const spec = row.original.spec
      const baseURL = spec.openai?.baseURL ?? spec.anthropic?.baseURL
      if (baseURL) return h('span', { class: 'text-sm font-mono truncate max-w-48 block' }, baseURL)
      if (spec.google) {
        const parts = [spec.google.project, spec.google.location].filter(Boolean)
        return parts.length ? h('span', { class: 'text-sm text-muted' }, parts.join(' / ')) : h('span', { class: 'text-muted' }, 'Default')
      }
      if (spec.bedrock?.region) return h('span', { class: 'text-sm text-muted' }, spec.bedrock.region)
      return h('span', { class: 'text-muted' }, 'Default')
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
  <UDashboardPanel id="providers">
    <template #header>
      <UDashboardNavbar title="Model Providers" icon="i-lucide-cloud">
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
          placeholder="Filter providers..."
        />

        <div class="flex flex-wrap items-center gap-1.5">
          <USelect
            v-model="providerTypeFilter"
            :items="[
              { label: 'All', value: 'all' },
              { label: 'OpenAI', value: 'openai' },
              { label: 'Anthropic', value: 'anthropic' },
              { label: 'Google', value: 'google' },
              { label: 'Bedrock', value: 'bedrock' }
            ]"
            :ui="{ trailingIcon: 'group-data-[state=open]:rotate-180 transition-transform duration-200' }"
            placeholder="Filter provider"
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
        v-if="providers.length === 0 && status !== 'pending'"
        icon="i-lucide-cloud"
        title="No providers configured"
        description="Set up a ModelProvider to connect to OpenAI, Anthropic, Google, or AWS Bedrock. Providers manage API keys and connection settings."
        docs-url="https://flokoa.ai/modelprovider"
        docs-label="Provider Guide"
      />

      <template v-else>
        <UTable
          ref="table"
          v-model:column-filters="columnFilters"
          v-model:column-visibility="columnVisibility"
          v-model:pagination="pagination"
          :pagination-options="{ getPaginationRowModel: getPaginationRowModel() }"
          class="shrink-0"
          :data="providers"
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
          @select="(_e: Event, row: { original: ModelProvider }) => openDetail(row.original)"
        />

        <div class="flex items-center justify-between gap-3 border-t border-default pt-4 mt-auto">
          <div class="text-sm text-muted">
            {{ table?.tableApi?.getFilteredRowModel().rows.length || 0 }} provider(s)
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

  <ProviderDetail v-if="selectedProvider" v-model:open="detailOpen" :provider="selectedProvider" />
</template>
