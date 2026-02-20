import { createSharedComposable } from '@vueuse/core'

const _useNamespace = () => {
  // Empty string means "all namespaces"
  const current = ref('')

  return {
    current
  }
}

export const useNamespace = createSharedComposable(_useNamespace)
