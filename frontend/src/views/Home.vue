<template>
  <div>
    <PhotoGrid :fetch-fn="wrapFetch" />
  </div>
</template>

<script setup lang="ts">
import PhotoGrid from '@/components/PhotoGrid.vue'
import { fetchPhotos } from '@/api/photos'

function wrapFetch(params: { limit: number; cursor?: string }, signal?: AbortSignal) {
  return fetchPhotos(params, signal)
}
</script>
