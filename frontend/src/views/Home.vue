<template>
  <div class="home">
    <h1>ShutterSeek</h1>
    <p>{{ status }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'

const status = ref('Loading...')

onMounted(async () => {
  try {
    const res = await fetch('/api/health')
    const data = await res.json()
    status.value = `API: ${data.status}`
  } catch {
    status.value = 'API unavailable'
  }
})
</script>

<style scoped>
.home {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
}
h1 { font-size: 2rem; color: #333; }
p { color: #666; margin-top: 1rem; }
</style>
