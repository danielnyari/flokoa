<script setup lang="ts">
const members = ref([
  { name: 'Admin', username: 'admin', role: 'owner' as const },
  { name: 'Operator', username: 'operator', role: 'member' as const }
])

const q = ref('')

const filteredMembers = computed(() => {
  return members.value.filter((member) => {
    return member.name.search(new RegExp(q.value, 'i')) !== -1 || member.username.search(new RegExp(q.value, 'i')) !== -1
  })
})
</script>

<template>
  <div>
    <UPageCard
      title="Members"
      description="Manage team members and their roles."
      variant="naked"
      orientation="horizontal"
      class="mb-4"
    >
      <UButton
        label="Invite people"
        color="neutral"
        class="w-fit lg:ms-auto"
      />
    </UPageCard>

    <UPageCard variant="subtle" :ui="{ container: 'p-0 sm:p-0 gap-y-0', wrapper: 'items-stretch', header: 'p-4 mb-0 border-b border-default' }">
      <template #header>
        <UInput
          v-model="q"
          icon="i-lucide-search"
          placeholder="Search members"
          autofocus
          class="w-full"
        />
      </template>

      <div class="divide-y divide-default">
        <div v-for="member in filteredMembers" :key="member.username" class="flex items-center justify-between p-4">
          <div class="flex items-center gap-3">
            <UAvatar :alt="member.name" size="sm" />
            <div>
              <p class="text-sm font-medium text-highlighted">
                {{ member.name }}
              </p>
              <p class="text-xs text-muted">
                @{{ member.username }}
              </p>
            </div>
          </div>
          <UBadge :color="member.role === 'owner' ? 'primary' : 'neutral'" variant="subtle" class="capitalize">
            {{ member.role }}
          </UBadge>
        </div>
      </div>
    </UPageCard>
  </div>
</template>
