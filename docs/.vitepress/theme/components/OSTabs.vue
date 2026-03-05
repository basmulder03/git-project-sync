<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

type OSType = 'linux' | 'windows'

const STORAGE_KEY = 'gps-docs-os-tab'
const EVENT_NAME = 'gps-docs-os-tab-change'

const selected = ref<OSType>('linux')

const applySelection = (value: string | null) => {
  if (value === 'linux' || value === 'windows') {
    selected.value = value
  }
}

const setSelected = (value: OSType) => {
  selected.value = value
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(STORAGE_KEY, value)
    window.dispatchEvent(new CustomEvent(EVENT_NAME, { detail: value }))
  }
}

const onExternalChange = (event: Event) => {
  const custom = event as CustomEvent<string>
  applySelection(custom.detail)
}

onMounted(() => {
  if (typeof window === 'undefined') {
    return
  }

  applySelection(window.localStorage.getItem(STORAGE_KEY))
  window.addEventListener(EVENT_NAME, onExternalChange)
})

onBeforeUnmount(() => {
  if (typeof window === 'undefined') {
    return
  }
  window.removeEventListener(EVENT_NAME, onExternalChange)
})
</script>

<template>
  <div class="os-tabs">
    <div class="os-tabs-controls" role="tablist" aria-label="Operating system tabs">
      <button
        class="os-tab"
        :class="{ active: selected === 'linux' }"
        role="tab"
        :aria-selected="selected === 'linux'"
        @click="setSelected('linux')"
      >
        Linux
      </button>
      <button
        class="os-tab"
        :class="{ active: selected === 'windows' }"
        role="tab"
        :aria-selected="selected === 'windows'"
        @click="setSelected('windows')"
      >
        Windows
      </button>
    </div>

    <div class="os-tabs-panel" role="tabpanel" v-if="selected === 'linux'">
      <slot name="linux" />
    </div>
    <div class="os-tabs-panel" role="tabpanel" v-else>
      <slot name="windows" />
    </div>
  </div>
</template>
