<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import AuthLayout from '@/components/AuthLayout.vue'
import CredentialsForm from '@/components/CredentialsForm.vue'
import { safeRedirect } from '@/router'

const route = useRoute()
const router = useRouter()
</script>

<template>
  <AuthLayout title="Sign in" description="Sign in to manage your proxy hosts.">
    <CredentialsForm mode="login" @done="router.replace(safeRedirect(route.query.redirect))" />
  </AuthLayout>
</template>
