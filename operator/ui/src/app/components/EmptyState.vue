<script setup lang="ts">
defineProps<{
  icon: string
  title: string
  description: string
  docsUrl?: string
  docsLabel?: string
}>()
</script>

<template>
  <div class="flex flex-col items-center justify-center py-16 px-4 text-center">
    <div class="flex items-center justify-center size-14 rounded-xl bg-primary/10 mb-4">
      <UIcon :name="icon" class="size-7 text-primary" />
    </div>
    <h3 class="text-base font-semibold text-highlighted mb-1">
      {{ title }}
    </h3>
    <p class="text-sm text-muted max-w-sm mb-4">
      {{ description }}
    </p>
    <div class="flex items-center gap-2">
      <UButton
        v-if="docsUrl"
        :label="docsLabel ?? 'View Documentation'"
        icon="i-lucide-book-open"
        color="neutral"
        variant="outline"
        size="sm"
        :to="docsUrl"
        target="_blank"
      />
      <slot />
    </div>
  </div>
</template>
