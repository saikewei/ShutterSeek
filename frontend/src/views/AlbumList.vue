<template>
  <div class="p-2">
    <p class="text-xs text-neutral-500 mb-2 px-1">{{ albums.length }} albums</p>
    <div v-if="loading" class="text-center text-neutral-500 py-12 text-sm">Loading...</div>
    <div v-else class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3">
      <div
        v-for="album in albums"
        :key="album.id"
        class="group cursor-pointer rounded-lg overflow-hidden bg-neutral-800 hover:ring-2 hover:ring-neutral-600 transition-all"
        @click="$router.push(`/albums/${album.id}`)"
      >
        <div class="aspect-square bg-neutral-700">
          <img
            v-if="album.cover_url"
            :src="album.cover_url"
            :alt="album.title"
            class="w-full h-full object-cover"
            loading="lazy"
          />
          <div v-else class="w-full h-full flex items-center justify-center text-neutral-500 text-4xl">📷</div>
        </div>
        <div class="p-3">
          <p class="text-sm font-medium text-white truncate">{{ album.title }}</p>
          <p class="text-xs text-neutral-400">{{ album.photo_count.toLocaleString() }} photos</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { fetchAlbums, type Album } from '@/api/albums'

const albums = ref<Album[]>([])
const loading = ref(true)

onMounted(async () => {
  try {
    const data = await fetchAlbums()
    albums.value = data.items
  } catch (e) {
    console.error('load albums failed', e)
  } finally {
    loading.value = false
  }
})
</script>
