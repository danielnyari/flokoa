import { createSharedComposable } from '@vueuse/core'

const _useDashboard = () => {
  const router = useRouter()

  defineShortcuts({
    'g-h': () => router.push('/'),
    'g-a': () => router.push('/agents'),
    'g-m': () => router.push('/models'),
    'g-p': () => router.push('/providers'),
    'g-t': () => router.push('/tools'),
    'g-w': () => router.push('/workflows'),
    'g-s': () => router.push('/settings')
  })

  return {}
}

export const useDashboard = createSharedComposable(_useDashboard)
