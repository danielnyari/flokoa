import { createSharedComposable } from '@vueuse/core'

// Sentinel value used in USelect items (Nuxt UI rejects empty-string values).
export const ALL_NAMESPACES = '__all__'

const _useNamespace = () => {
  // The select binds to `selected`; consumers read `current` which maps
  // the sentinel back to an empty string (meaning "all namespaces").
  const selected = ref(ALL_NAMESPACES)
  const current = computed(() => (selected.value === ALL_NAMESPACES ? '' : selected.value))

  return {
    selected,
    current
  }
}

export const useNamespace = createSharedComposable(_useNamespace)
