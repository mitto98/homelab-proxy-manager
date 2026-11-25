<script setup lang="ts">
import { useMutation, useQueryClient } from '@tanstack/vue-query'
import { toTypedSchema } from '@vee-validate/zod'
import { useForm } from 'vee-validate'
import { ref } from 'vue'
import { z } from 'zod'
import { api, ApiError, type User } from '@/api/client'
import { Button } from '@/components/ui/button'
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { statusKey } from '@/query'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{ mode: 'setup' | 'login' }>()
const emit = defineEmits<{ done: [user: User] }>()

// Mirrors the Credentials schema in api/openapi.yaml; the server stays the
// final validator.
const credentials = z.object({
  username: z.string().trim().min(1, 'Enter a username').max(64, 'At most 64 characters'),
  password: z.string().min(8, 'At least 8 characters').max(72, 'At most 72 characters'),
})
const schema = toTypedSchema(
  props.mode === 'setup'
    ? credentials.extend({ confirm: z.string() }).refine((v) => v.password === v.confirm, {
        path: ['confirm'],
        message: 'Passwords do not match',
      })
    : credentials,
)

const { handleSubmit, setFieldError } = useForm({ validationSchema: schema })
const auth = useAuthStore()
const qc = useQueryClient()
const formError = ref('')

const mutation = useMutation({
  mutationFn: (values: { username: string; password: string }) =>
    props.mode === 'setup' ? api.setup(values) : api.login(values),
  onSuccess(user) {
    auth.setUser(user)
    if (props.mode === 'setup') qc.setQueryData(statusKey, { setupRequired: false, version: '' })
    emit('done', user)
  },
  onError(e) {
    if (
      e instanceof ApiError &&
      e.status === 400 &&
      (e.field === 'username' || e.field === 'password')
    ) {
      setFieldError(e.field, e.message)
    } else if (e instanceof ApiError && e.status !== 0 && e.status < 500) {
      formError.value = e.message
    } else {
      formError.value = 'Something went wrong. Try again.'
    }
  },
})

const onSubmit = handleSubmit((values) => {
  formError.value = ''
  mutation.mutate({ username: values.username, password: values.password })
})
</script>

<template>
  <form class="grid gap-4" novalidate @submit.prevent="onSubmit">
    <FormField v-slot="{ componentField }" name="username">
      <FormItem>
        <FormLabel>Username</FormLabel>
        <FormControl>
          <Input type="text" autocomplete="username" autofocus v-bind="componentField" />
        </FormControl>
        <FormMessage />
      </FormItem>
    </FormField>
    <FormField v-slot="{ componentField }" name="password">
      <FormItem>
        <FormLabel>Password</FormLabel>
        <FormControl>
          <Input
            type="password"
            :autocomplete="mode === 'setup' ? 'new-password' : 'current-password'"
            v-bind="componentField"
          />
        </FormControl>
        <FormMessage />
      </FormItem>
    </FormField>
    <FormField v-if="mode === 'setup'" v-slot="{ componentField }" name="confirm">
      <FormItem>
        <FormLabel>Confirm password</FormLabel>
        <FormControl>
          <Input type="password" autocomplete="new-password" v-bind="componentField" />
        </FormControl>
        <FormMessage />
      </FormItem>
    </FormField>
    <p v-if="formError" role="alert" class="text-destructive text-sm">{{ formError }}</p>
    <Button type="submit" :disabled="mutation.isPending.value">
      {{ mode === 'setup' ? 'Create admin' : 'Sign in' }}
    </Button>
  </form>
</template>
