import { VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import { createApp } from 'vue'
import { createWebHistory } from 'vue-router'
import App from './App.vue'
import { api } from './api/client'
import { createQueryClient } from './query'
import { createAppRouter } from './router'
import './style.css'

const queryClient = createQueryClient()
const app = createApp(App)
app.use(createPinia())
app.use(VueQueryPlugin, { queryClient })
app.use(createAppRouter(createWebHistory(), queryClient, api))
app.mount('#app')
