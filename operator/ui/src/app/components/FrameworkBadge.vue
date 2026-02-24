<script setup lang="ts">
import { frameworkInfo } from '~/utils/enums'

const props = defineProps<{
  value?: string | number | null
}>()

const info = computed(() => frameworkInfo(props.value))
const hasLogo = computed(() => info.value?.logoLight || info.value?.logoDark)
</script>

<template>
  <span v-if="info" class="inline-flex items-center">
    <!-- Light/dark logo variants -->
    <template v-if="hasLogo">
      <a
        v-if="info.url"
        :href="info.url"
        target="_blank"
        rel="noopener noreferrer"
        class="shrink-0 hover:opacity-75 transition-opacity"
      >
        <img v-if="info.logoLight" :src="info.logoLight" :alt="info.label" class="h-4 dark:hidden">
        <img v-if="info.logoDark" :src="info.logoDark" :alt="info.label" class="h-4 hidden dark:block">
      </a>
      <template v-else>
        <img v-if="info.logoLight" :src="info.logoLight" :alt="info.label" class="h-4 shrink-0 dark:hidden">
        <img v-if="info.logoDark" :src="info.logoDark" :alt="info.label" class="h-4 shrink-0 hidden dark:block">
      </template>
    </template>
    <!-- Fallback: first letter -->
    <span v-else class="flex items-center justify-center size-4 rounded bg-muted/20 text-[8px] font-bold text-muted shrink-0">
      {{ info.label.charAt(0) }}
    </span>
  </span>
  <span v-else class="text-muted">&mdash;</span>
</template>
