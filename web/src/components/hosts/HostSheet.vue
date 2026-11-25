<script setup lang="ts">
import { ref } from 'vue'
import type { Host } from '@/api/client'
import HostForm from '@/components/hosts/HostForm.vue'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Sheet, SheetContent } from '@/components/ui/sheet'

defineProps<{ open: boolean; host: Host | null }>()
const emit = defineEmits<{ close: [] }>()

// The form lives inside the sheet content, so it is created fresh on every
// open and its state is gone on close.
const form = ref<InstanceType<typeof HostForm> | null>(null)
const confirmDiscard = ref(false)

// Esc, a click on the overlay and the close button all arrive here. Do not
// lose typed input silently.
function onOpenChange(next: boolean) {
  if (next) return
  if (form.value?.dirty) confirmDiscard.value = true
  else emit('close')
}
</script>

<template>
  <Sheet :open="open" @update:open="onOpenChange">
    <SheetContent class="gap-0 sm:max-w-xl">
      <HostForm ref="form" :host="host" @close="emit('close')" />
    </SheetContent>
  </Sheet>
  <AlertDialog v-model:open="confirmDiscard">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>Discard your changes?</AlertDialogTitle>
        <AlertDialogDescription>
          You have unsaved changes. Closing the form loses them.
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel>Keep editing</AlertDialogCancel>
        <AlertDialogAction @click="emit('close')">Discard</AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
