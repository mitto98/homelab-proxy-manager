<script setup lang="ts">
import { toTypedSchema } from '@vee-validate/zod'
import { useForm } from 'vee-validate'
import { computed, ref } from 'vue'
import { ApiError, type Host } from '@/api/client'
import FormSection from '@/components/FormSection.vue'
import DomainsInput from '@/components/hosts/DomainsInput.vue'
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
import { Button } from '@/components/ui/button'
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { useCreateHost, useDeleteHost, useUpdateHost } from '@/hosts/queries'
import {
  formValuesToInput,
  hostFormFields,
  hostFormSchema,
  hostToFormValues,
  type HostFormField,
} from '@/hosts/schema'
import { describeHost } from '@/hosts/summary'

const props = defineProps<{ host: Host | null }>()
// `close` means "leave the form": after a save, a delete or Cancel. Closing
// with unsaved input (Esc, overlay) is the sheet's business; it asks first.
const emit = defineEmits<{ close: [] }>()

const { handleSubmit, setFieldError, setFieldValue, meta, values } = useForm({
  validationSchema: toTypedSchema(hostFormSchema),
  initialValues: hostToFormValues(props.host),
  // The skip-verify switch is only rendered for https. Without this, unmounting
  // it would drop its value and the schema would reject the missing boolean.
  keepValuesOnUnmount: true,
})

const create = useCreateHost()
const update = useUpdateHost()
const remove = useDeleteHost()
const saving = computed(() => create.isPending.value || update.isPending.value)
const busy = computed(() => saving.value || remove.isPending.value)

// Save needs something to save: vee-validate compares against the initial
// values, so reverting every change makes the form clean again.
const dirty = computed(() => meta.value.dirty)
const summary = computed(() => describeHost(values))
const formError = ref('')
const confirmDelete = ref(false)

function isFormField(field: string | undefined): field is HostFormField {
  return (hostFormFields as readonly string[]).includes(field ?? '')
}

// The server is the final validator. A 400 names the field it is about, a 409
// is a domain already used by another host; both show on the form's field. Any
// other failure is a message inside the form, which stays open with its input.
function showError(e: unknown) {
  if (e instanceof ApiError && e.status === 400 && isFormField(e.field)) {
    setFieldError(e.field, e.message)
  } else if (e instanceof ApiError && e.status === 409) {
    setFieldError('domains', e.message)
  } else if (e instanceof ApiError && e.status >= 400 && e.status < 500) {
    formError.value = e.message
  } else {
    formError.value = 'Something went wrong. Try again.'
  }
}

const onSubmit = handleSubmit((v) => {
  formError.value = ''
  const input = formValuesToInput(v)
  const done = { onSuccess: () => emit('close'), onError: showError }
  if (props.host) update.mutate({ id: props.host.id, input }, done)
  else create.mutate(input, done)
})

function deleteHost() {
  if (!props.host) return
  formError.value = ''
  remove.mutate(props.host.id, { onSuccess: () => emit('close'), onError: showError })
}

function setScheme(value: unknown) {
  setFieldValue('upstreamScheme', value as 'http' | 'https')
  // Skipping verification only means something for https.
  if (value === 'http') setFieldValue('upstreamSkipTlsVerify', false)
}

defineExpose({ dirty })
</script>

<template>
  <form class="flex min-h-0 flex-1 flex-col" novalidate @submit.prevent="onSubmit">
    <SheetHeader class="border-b p-6 pr-12">
      <SheetTitle>{{ host ? 'Edit Proxy Host' : 'Add Proxy Host' }}</SheetTitle>
      <SheetDescription>
        {{
          host
            ? 'Change where this host sends requests.'
            : 'Forward one or more domain names to a service on your network.'
        }}
      </SheetDescription>
    </SheetHeader>

    <div class="bg-muted/40 flex-1 space-y-5 overflow-y-auto p-6">
      <FormSection
        title="Details"
        description="The public names, and the service that answers for them."
      >
        <FormField v-slot="{ componentField }" name="domains">
          <FormItem>
            <FormLabel>Domain names</FormLabel>
            <FormControl>
              <DomainsInput v-bind="componentField" />
            </FormControl>
            <p class="text-muted-foreground text-sm">
              Type a name and press Enter, comma or space.
            </p>
            <FormMessage />
          </FormItem>
        </FormField>

        <div class="grid grid-cols-[7rem_1fr_6.5rem] gap-3">
          <FormField v-slot="{ value }" name="upstreamScheme">
            <FormItem>
              <FormLabel>Scheme</FormLabel>
              <FormControl>
                <NativeSelect :model-value="value" @update:model-value="setScheme">
                  <NativeSelectOption value="http">http</NativeSelectOption>
                  <NativeSelectOption value="https">https</NativeSelectOption>
                </NativeSelect>
              </FormControl>
              <FormMessage />
            </FormItem>
          </FormField>
          <FormField v-slot="{ componentField }" name="upstreamHost">
            <FormItem>
              <FormLabel>Upstream host</FormLabel>
              <FormControl>
                <Input
                  type="text"
                  placeholder="192.168.1.20"
                  autocomplete="off"
                  v-bind="componentField"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          </FormField>
          <FormField v-slot="{ componentField }" name="upstreamPort">
            <FormItem>
              <FormLabel>Port</FormLabel>
              <FormControl>
                <Input type="number" min="1" max="65535" v-bind="componentField" />
              </FormControl>
              <FormMessage />
            </FormItem>
          </FormField>
        </div>

        <FormField
          v-if="values.upstreamScheme === 'https'"
          v-slot="{ value, handleChange }"
          name="upstreamSkipTlsVerify"
        >
          <FormItem class="flex items-center gap-3">
            <FormControl>
              <Switch :model-value="value" @update:model-value="handleChange" />
            </FormControl>
            <FormLabel>Skip upstream TLS verification</FormLabel>
          </FormItem>
        </FormField>

        <FormField v-slot="{ value, handleChange }" name="enabled">
          <FormItem class="flex items-center gap-3">
            <FormControl>
              <Switch :model-value="value" @update:model-value="handleChange" />
            </FormControl>
            <FormLabel>Enabled</FormLabel>
          </FormItem>
        </FormField>
      </FormSection>

      <!-- [3.2] adds an SSL FormSection here and [5.3] an Access one. -->

      <FormSection
        title="What will happen"
        description="What Traefik will do with this host, as the form stands."
        class="border-dashed"
      >
        <ul class="space-y-2 text-sm" data-testid="summary">
          <li v-for="line in summary" :key="line">{{ line }}</li>
        </ul>
      </FormSection>

      <p v-if="formError" role="alert" class="text-destructive text-sm">{{ formError }}</p>
    </div>

    <div class="flex items-center gap-2 border-t p-4">
      <Button
        v-if="host"
        type="button"
        variant="outline"
        class="text-destructive hover:text-destructive"
        :disabled="busy"
        @click="confirmDelete = true"
      >
        Delete
      </Button>
      <span
        v-if="dirty"
        role="status"
        class="flex items-center gap-1.5 text-sm text-amber-600 dark:text-amber-400"
      >
        <span class="size-2 rounded-full bg-current" aria-hidden="true" />
        Unsaved changes
      </span>
      <div class="ml-auto flex gap-2">
        <Button type="button" variant="outline" :disabled="busy" @click="emit('close')"
          >Cancel</Button
        >
        <Button type="submit" :disabled="!dirty || busy">Save</Button>
      </div>
    </div>

    <AlertDialog v-model:open="confirmDelete">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete this proxy host?</AlertDialogTitle>
          <AlertDialogDescription>
            Traefik stops routing {{ host?.domains.join(', ') }}. This cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Keep host</AlertDialogCancel>
          <AlertDialogAction @click="deleteHost">Delete host</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </form>
</template>
