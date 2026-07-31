<template>
  <div class="p-3">
    <div class="flex items-center justify-between mb-3">
      <p class="text-xs text-neutral-500">{{ albums.length }} albums</p>
      <button v-if="isAdmin" @click="openCreate" class="px-3 py-1.5 text-xs rounded-full bg-neutral-700 text-white hover:bg-neutral-600 transition-colors">+ 新建相册</button>
    </div>

    <div v-if="loading" class="text-center text-neutral-500 py-12 text-sm">Loading...</div>
    <div v-else class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3">
      <div
        v-for="album in albums"
        :key="album.id"
        class="group relative cursor-pointer rounded-lg overflow-hidden bg-neutral-800 hover:ring-2 hover:ring-neutral-600 transition-all"
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

        <!-- Context menu button -->
        <button
          v-if="isAdmin"
          class="absolute top-1.5 right-1.5 w-7 h-7 rounded-full bg-black/60 text-neutral-300 hover:text-white hover:bg-black/80 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center text-sm"
          @click.stop="toggleMenu(album)"
        >⋯</button>

        <!-- Inline context menu -->
        <div v-if="menuAlbum === album" class="absolute inset-0 z-10 bg-neutral-900/95 rounded-lg flex flex-col items-center justify-center gap-1" @click.stop>
          <button @click.stop="openRename(album)" class="px-4 py-1.5 text-sm text-neutral-300 hover:text-white transition-colors">重命名</button>
          <button @click.stop="togglePublic(album)" class="px-4 py-1.5 text-sm text-neutral-300 hover:text-white transition-colors">
            {{ album.is_public ? '设为私有' : '设为公开' }}
          </button>
          <button @click.stop="confirmDelete(album)" class="px-4 py-1.5 text-sm text-red-400 hover:text-red-300 transition-colors">删除</button>
          <button @click.stop="menuAlbum = null" class="mt-2 text-xs text-neutral-500 hover:text-neutral-400">取消</button>
        </div>

        <div class="p-3">
          <p class="text-sm font-medium text-white truncate flex items-center gap-1">
            <span>{{ album.title }}</span>
            <span v-if="album.is_public" class="text-[10px] text-emerald-400" title="公开相册">🔓</span>
            <span v-else class="text-[10px] text-neutral-500" title="私有相册">🔒</span>
          </p>
          <p class="text-xs text-neutral-400">{{ album.photo_count.toLocaleString() }} photos</p>
        </div>
      </div>
    </div>

    <!-- Create / Rename dialog -->
    <Teleport to="body">
      <div v-if="dialog.open" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="absolute inset-0 bg-black/60" @click="closeDialog" />
        <div class="relative bg-neutral-800 rounded-xl p-5 w-80 shadow-xl border border-neutral-700">
          <h2 class="text-sm font-medium text-white mb-3">{{ dialog.mode === 'create' ? '新建相册' : '重命名' }}</h2>
          <input
            ref="titleInput"
            v-model="dialog.title"
            @keyup.enter="confirmDialog"
            class="w-full px-3 py-2 text-sm rounded-lg bg-neutral-700 text-white border border-neutral-600 outline-none focus:border-neutral-400 mb-2"
            placeholder="相册名称"
          />
          <textarea
            v-if="dialog.mode === 'create'"
            v-model="dialog.description"
            class="w-full px-3 py-2 text-sm rounded-lg bg-neutral-700 text-white border border-neutral-600 outline-none focus:border-neutral-400 resize-none h-16"
            placeholder="描述（可选）"
          />
          <label class="flex items-center gap-2 cursor-pointer mt-1">
            <input type="checkbox" v-model="dialog.isPublic" class="rounded accent-white" />
            <span class="text-xs text-neutral-300">公开（访客可见）</span>
          </label>
          <div class="flex justify-end gap-2 mt-3">
            <button @click="closeDialog" class="px-3 py-1.5 text-xs rounded-full text-neutral-400 hover:text-white transition-colors">取消</button>
            <button @click="confirmDialog" class="px-4 py-1.5 text-xs rounded-full bg-white text-black font-medium hover:bg-neutral-200 transition-colors">确定</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Delete confirmation -->
    <Teleport to="body">
      <div v-if="deleteTarget" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="absolute inset-0 bg-black/60" @click="deleteTarget = null" />
        <div class="relative bg-neutral-800 rounded-xl p-5 w-80 shadow-xl border border-neutral-700">
          <h2 class="text-sm font-medium text-white mb-1">删除相册</h2>
          <p class="text-xs text-neutral-400 mb-4">确定要删除「{{ deleteTarget.title }}」吗？此操作不可撤销。</p>
          <div class="flex justify-end gap-2">
            <button @click="deleteTarget = null" class="px-3 py-1.5 text-xs rounded-full text-neutral-400 hover:text-white">取消</button>
            <button @click="doDelete" class="px-4 py-1.5 text-xs rounded-full bg-red-600 text-white font-medium hover:bg-red-500">删除</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { fetchAlbums, createAlbum, updateAlbum, deleteAlbum, type Album } from '@/api/albums'
import { isAdmin } from '@/stores/auth'

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
  menuAlbum.value = null
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

// ── Context menu ──────────────────────────────────────

const menuAlbum = ref<Album | null>(null)

function toggleMenu(album: Album) {
  menuAlbum.value = menuAlbum.value === album ? null : album
}

// ── Delete ────────────────────────────────────────────

const deleteTarget = ref<Album | null>(null)

function confirmDelete(album: Album) {
  menuAlbum.value = null
  deleteTarget.value = album
}

async function doDelete() {
  if (!deleteTarget.value) return
  await deleteAlbum(deleteTarget.value.id)
  deleteTarget.value = null
  await load()
}

// ── Public toggle ─────────────────────────────────────

async function togglePublic(album: Album) {
  menuAlbum.value = null
  try {
    await updateAlbum(album.id, { is_public: !album.is_public })
    await load()
  } catch { /* backend rejects */ }
}
</script>
