<template>
  <div class="p-3">
    <div class="flex items-center justify-between mb-3">
      <p class="text-xs text-ink-3">{{ albums.length }} 个相册</p>
      <button v-if="isAdmin" @click="openCreate" class="btn-primary px-3 py-1.5 text-xs">+ 新建相册</button>
    </div>

    <div v-if="loading" class="text-center text-ink-3 py-12 text-sm">加载中...</div>
    <div v-else class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3">
      <div
        v-for="album in albums"
        :key="album.id"
        class="group relative cursor-pointer rounded-lg overflow-hidden bg-surface hover:ring-2 hover:ring-line-strong transition-all"
        @click="$router.push(`/albums/${album.id}`)"
      >
        <div class="aspect-square bg-raised">
          <img
            v-if="album.cover_url"
            :src="album.cover_url"
            :alt="album.title"
            class="w-full h-full object-cover"
            loading="lazy"
          />
          <div v-else class="w-full h-full flex items-center justify-center text-ink-3 text-4xl">📷</div>
        </div>

        <!-- Touch has no hover, so the actions entry stays visible there -->
        <button
          v-if="isAdmin"
          class="absolute top-2 right-2 w-8 h-8 rounded-full bg-black/60 text-ink hover:bg-black/80 backdrop-blur-[2px] transition-opacity duration-150 flex items-center justify-center text-sm opacity-100 md:opacity-0 md:group-hover:opacity-100"
          aria-label="相册操作"
          @click.stop="sheetAlbum = album"
        >⋯</button>

        <div class="p-2.5">
          <p class="text-sm font-medium text-ink truncate flex items-center gap-1">
            <span class="truncate">{{ album.title }}</span>
            <span v-if="album.is_public" class="text-[10px] text-success shrink-0" title="公开相册">🔓</span>
            <span v-else class="text-[10px] text-ink-3 shrink-0" title="私有相册">🔒</span>
          </p>
          <p class="text-xs text-ink-2 tabular-nums">{{ album.photo_count.toLocaleString() }} photos</p>
        </div>
      </div>
    </div>

    <ActionSheet
      :open="sheetAlbum !== null"
      :title="sheetAlbum?.title || ''"
      :actions="sheetActions"
      @close="sheetAlbum = null"
      @select="onSheetSelect"
    />

    <!-- Create / Rename dialog -->
    <AppModal :open="dialog.open" :title="dialog.mode === 'create' ? '新建相册' : '重命名'" @close="closeDialog">
      <input
        ref="titleInput"
        v-model="dialog.title"
        @keyup.enter="confirmDialog"
        class="input mb-2"
        placeholder="相册名称"
      />
      <textarea
        v-if="dialog.mode === 'create'"
        v-model="dialog.description"
        class="input resize-none h-16"
        placeholder="描述（可选）"
      />
      <label class="flex items-center gap-2 cursor-pointer mt-1">
        <input type="checkbox" v-model="dialog.isPublic" class="rounded accent-accent" />
        <span class="text-xs text-ink-2">公开（访客可见）</span>
      </label>

      <template #footer>
        <div class="flex justify-end gap-2">
          <button @click="closeDialog" class="px-3 py-1.5 text-xs rounded-full text-ink-3 hover:text-ink transition-colors">取消</button>
          <button @click="confirmDialog" class="px-4 py-1.5 text-xs rounded-full bg-accent text-[#1C1208] font-semibold hover:bg-accent-strong transition-colors">确定</button>
        </div>
      </template>
    </AppModal>

    <!-- Delete confirmation -->
    <AppModal :open="deleteTarget !== null" title="删除相册" @close="deleteTarget = null">
      <p class="text-xs text-ink-2 mb-4">确定要删除「{{ deleteTarget?.title }}」吗？此操作不可撤销。</p>
      <div class="flex justify-end gap-2">
        <button @click="deleteTarget = null" class="px-3 py-1.5 text-xs rounded-full text-ink-3 hover:text-ink">取消</button>
        <button @click="doDelete" class="btn-danger px-4 py-1.5 text-xs">删除</button>
      </div>
    </AppModal>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, nextTick } from 'vue'
import { fetchAlbums, createAlbum, updateAlbum, deleteAlbum, type Album } from '@/api/albums'
import { isAdmin } from '@/stores/auth'
import AppModal from '@/components/AppModal.vue'
import ActionSheet, { type SheetAction } from '@/components/ActionSheet.vue'

const albums = ref<Album[]>([])
const loading = ref(true)

async function load() {
  try { albums.value = (await fetchAlbums()).items } catch (e) { console.error(e) }
  finally { loading.value = false }
}

onMounted(load)

// ── Create / Rename dialog ────────────────────────────

const dialog = ref<{ open: boolean; mode: 'create' | 'rename'; title: string; description: string; isPublic: boolean; albumId: number | null }>({
  open: false, mode: 'create', title: '', description: '', isPublic: false, albumId: null
})
const titleInput = ref<HTMLInputElement | null>(null)

function openCreate() {
  dialog.value = { open: true, mode: 'create', title: '', description: '', isPublic: false, albumId: null }
  nextTick(() => titleInput.value?.focus())
}

function openRename(album: Album) {
  dialog.value = { open: true, mode: 'rename', title: album.title, description: '', isPublic: album.is_public, albumId: album.id }
  nextTick(() => titleInput.value?.focus())
}

function closeDialog() { dialog.value.open = false }

async function confirmDialog() {
  const t = dialog.value.title.trim()
  if (!t) return
  if (dialog.value.mode === 'create') {
    await createAlbum(t, dialog.value.description, dialog.value.isPublic)
  } else if (dialog.value.albumId) {
    await updateAlbum(dialog.value.albumId, { title: t, is_public: dialog.value.isPublic })
  }
  dialog.value.open = false
  await load()
}

// ── Per-album actions ─────────────────────────────────
// Touch has no hover, so the entry point is an always-visible button on mobile
// and an action sheet rather than an inline menu.

const sheetAlbum = ref<Album | null>(null)

const sheetActions = computed<SheetAction[]>(() => {
  const album = sheetAlbum.value
  if (!album) return []
  return [
    { key: 'rename', label: '重命名' },
    { key: 'toggle', label: album.is_public ? '设为私有' : '设为公开' },
    { key: 'delete', label: '删除相册', danger: true },
  ]
})

function onSheetSelect(key: string) {
  const album = sheetAlbum.value
  if (!album) return
  if (key === 'rename') openRename(album)
  else if (key === 'toggle') togglePublic(album)
  else if (key === 'delete') deleteTarget.value = album
}

// ── Delete ────────────────────────────────────────────

const deleteTarget = ref<Album | null>(null)

async function doDelete() {
  if (!deleteTarget.value) return
  await deleteAlbum(deleteTarget.value.id)
  deleteTarget.value = null
  await load()
}

// ── Public toggle ─────────────────────────────────────

async function togglePublic(album: Album) {
  try {
    await updateAlbum(album.id, { is_public: !album.is_public })
    await load()
  } catch { /* backend rejects */ }
}
</script>
