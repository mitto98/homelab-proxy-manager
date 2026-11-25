<script setup lang="ts">
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

defineProps<{ title: string; description: string }>()
</script>

<template>
  <main class="bg-muted/40 flex min-h-screen items-center justify-center p-4">
    <Card class="w-full max-w-sm">
      <CardHeader>
        <p class="text-muted-foreground text-xs font-semibold uppercase">Proxy Manager</p>
        <CardTitle class="text-xl">{{ title }}</CardTitle>
        <CardDescription>{{ description }}</CardDescription>
      </CardHeader>
      <CardContent><slot /></CardContent>
    </Card>
  </main>
</template>
