<script setup lang="ts">
import { useRouter } from 'vue-router'
import AuthLayout from '@/components/AuthLayout.vue'
import CredentialsForm from '@/components/CredentialsForm.vue'

const router = useRouter()
</script>

<template>
  <AuthLayout title="Welcome" description="Create the first administrator account to get started.">
    <CredentialsForm mode="setup" @done="router.replace('/')" />
  </AuthLayout>
</template>
