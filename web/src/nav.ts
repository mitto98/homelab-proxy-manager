import { Network } from '@lucide/vue'
import type { Component } from 'vue'

export interface NavItem {
  label: string
  to: string
  icon: Component
}

// Only pages that exist. Later slices add their entry here.
export const navItems: NavItem[] = [{ label: 'Proxy Hosts', to: '/hosts', icon: Network }]
