<script setup lang="ts">
import { Monitor, Moon, Sun, LogOut } from '@lucide/vue'
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import type { Component } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { api } from '@/api/client'
import { Button } from '@/components/ui/button'
import { navItems } from '@/nav'
import { useAuthStore } from '@/stores/auth'
import { usePreferencesStore, type ThemeMode } from '@/stores/preferences'

const auth = useAuthStore()
const prefs = usePreferencesStore()
const route = useRoute()
const router = useRouter()
const qc = useQueryClient()

const themes: { mode: ThemeMode; label: string; icon: Component }[] = [
  { mode: 'system', label: 'System theme', icon: Monitor },
  { mode: 'light', label: 'Light theme', icon: Sun },
  { mode: 'dark', label: 'Dark theme', icon: Moon },
]

const signOut = useMutation({
  mutationFn: api.logout,
  async onSuccess() {
    auth.setUser(null)
    qc.clear()
    await router.replace({ name: 'login' })
  },
})
</script>

<template>
  <div class="flex min-h-screen">
    <aside
      class="bg-sidebar text-sidebar-foreground border-sidebar-border flex w-60 shrink-0 flex-col border-r"
    >
      <div class="px-5 py-4 text-lg font-semibold">Proxy Manager</div>
      <nav aria-label="Main" class="flex-1 space-y-1 px-3">
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="hover:bg-sidebar-accent text-muted-foreground flex items-center gap-2 rounded-md px-3 py-2 text-sm"
          active-class="!bg-sidebar-primary !text-sidebar-primary-foreground"
        >
          <component :is="item.icon" class="size-4" />
          {{ item.label }}
        </RouterLink>
      </nav>
      <div class="border-sidebar-border space-y-3 border-t p-4 text-sm">
        <div class="flex items-center justify-between">
          <span class="truncate font-medium" data-testid="user-name">{{
            auth.user?.username
          }}</span>
          <span class="text-muted-foreground text-xs">{{ auth.user?.role }}</span>
        </div>
        <div role="group" aria-label="Theme" class="flex gap-1">
          <Button
            v-for="t in themes"
            :key="t.mode"
            variant="ghost"
            size="icon-sm"
            :aria-label="t.label"
            :aria-pressed="prefs.mode === t.mode"
            :class="prefs.mode === t.mode && 'bg-accent'"
            @click="prefs.setMode(t.mode)"
          >
            <component :is="t.icon" />
          </Button>
        </div>
        <Button
          variant="outline"
          size="sm"
          class="w-full"
          :disabled="signOut.isPending.value"
          @click="signOut.mutate()"
        >
          <LogOut /> Sign out
        </Button>
      </div>
    </aside>
    <main class="flex-1 p-8">
      <h1 class="mb-6 text-2xl font-semibold">{{ route.meta.title }}</h1>
      <RouterView />
    </main>
  </div>
</template>
