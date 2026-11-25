<script setup lang="ts">
import { computed, ref, useAttrs } from 'vue'
import {
  TagsInput,
  TagsInputInput,
  TagsInputItem,
  TagsInputItemDelete,
  TagsInputItemText,
} from '@/components/ui/tags-input'

// Chips for domain names. Enter, comma or space adds what was typed (so does
// leaving the field or pasting a list); names are trimmed and lower-cased, and a
// name already in the list is refused. Attributes from FormControl (id, aria-*)
// go to the elements they belong on.
defineOptions({ inheritAttrs: false })
const model = defineModel<string[]>({ default: () => [] })
const attrs = useAttrs()
const duplicate = ref('')
const inputId = computed(() => attrs.id as string | undefined)
const describedBy = computed(() => attrs['aria-describedby'] as string | undefined)
const invalid = computed(() => attrs['aria-invalid'] as boolean | undefined)
const normalize = (v: string) => v.trim().toLowerCase()

function update(value: unknown) {
  duplicate.value = ''
  // A blank entry (spaces then blur) comes back as '': never keep it.
  model.value = (Array.isArray(value) ? (value as string[]) : []).filter((d) => d !== '')
}
</script>

<template>
  <div class="grid gap-1.5">
    <TagsInput
      :model-value="model"
      :delimiter="/[,\s]/"
      add-on-blur
      add-on-paste
      :convert-value="normalize"
      :aria-invalid="invalid"
      @update:model-value="update"
      @invalid="(v) => (duplicate = String(v))"
    >
      <TagsInputItem v-for="domain in model" :key="domain" :value="domain" class="h-6 gap-1 pl-2">
        <TagsInputItemText class="text-sm" />
        <TagsInputItemDelete :aria-label="`Remove ${domain}`" class="mr-0.5 p-0.5" />
      </TagsInputItem>
      <TagsInputInput
        :id="inputId"
        :aria-describedby="describedBy"
        placeholder="app.example.com, then Enter"
        class="min-w-40 flex-1 bg-transparent px-1 py-1 text-sm outline-none"
      />
    </TagsInput>
    <p v-if="duplicate" role="status" class="text-muted-foreground text-sm">
      {{ duplicate }} is already in the list.
    </p>
  </div>
</template>
