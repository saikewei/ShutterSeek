<template>
  <div class="p-3">
    <div class="flex items-center justify-between mb-3">
      <p class="text-xs text-ink-3">{{ albums.length }} albums</p>
      <button v-if="isAdmin" @click="openCreate" class="btn-primary px-3 py-1.5 text-xs">+ 新建相册</button>
    </div>

    <div v-if="loading" class="text-center text-ink-3 py-12 text-sm">Loading...</div>
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

        <!-- Context menu button -->
        <button
          v-if="isAdmin"
          class="absolute top-1.5 right-1.5 w-7 h-7 rounded-full bg-black/60 text-ink-2 hover:text-ink hover:bg-black/80 backdrop-blur-[2px] opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center text-sm"
          @click.stop="toggleMenu(album)"
        >⋯</button>

        <!-- Inline context menu -->
        <div v-if="menuAlbum === album" class="absolute inset-0 z-10 bg-base/95 backdrop-blur rounded-lg flex flex-col items-center justify-center gap-1" @click.stop>
          <button @click.stop="openRename(album)" class="px-4 py-1.5 text-sm text-ink-2 hover:text-ink transition-colors">重命名</button>
          <button @click.stop="togglePublic(album)" class="px-4 py-1.5 text-sm text-ink-2 hover:text-ink transition-colors">
            {{ album.is_public ? '设为私有' : '设为公开' }}
          </button>
          <button @click.stop="confirmDelete(album)" class="px-4 py-1.5 text-sm text-danger-ink hover:text-danger transition-colors">删除</button>
          <button @click.stop="menuAlbum = null" class="mt-2 text-xs text-ink-3 hover:text-ink-2">取消</button>
        </div>

        <div class="p-3">
          <p class="text-sm font-medium text-ink truncate flex items-center gap-1">
            <span>{{ album.title }}</span>
            <span v-if="album.is_public" class="text-[10px] text-success" title="公开相册">🔓</span>
            <span v-else class="text-[10px] text-ink-3" title="私有相册">🔒</span>
          </p>
          <p class="text-xs text-ink-2">{{ album.photo_count.toLocaleString() }} photos</p>
        </div>
      </div>
    </div>

    <!-- Create / Rename dialog -->
    <Teleport to="body">
      <div v-if="dialog.open" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="modal-overlay" @click="closeDialog" />
        <div class="modal-panel w-80">
          <h2 class="text-sm font-medium text-ink mb-3">{{ dialog.mode === 'create' ? '新建相册' : '重命名' }}</h2>
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
          <div class="flex justify-end gap-2 mt-3">
            <button @click="closeDialog" class="px-3 py-1.5 text-xs rounded-full text-ink-3 hover:text-ink transition-colors">取消</button>
            <button @click="confirmDialog" class="px-4 py-1.5 text-xs rounded-full bg-accent text-[#1C1208] font-semibold hover:bg-accent-strong transition-colors">确定</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Delete confirmation -->
    <Teleport to="body">
      <div v-if="deleteTarget" class="fixed inset-0 z-50 flex items-center justify-center">
        <div class="modal-overlay" @click="deleteTarget = null" />
        <div class="modal-panel w-80">
          <h2 class="text-sm font-medium text-ink mb-1">删除相册</h2>
          <p class="text-xs text-ink-2 mb-4">确定要删除「{{ deleteTarget.title }}」吗？此操作不可撤销。</p>
          <div class="flex justify-end gap-2">
            <button @click="deleteTarget = null" class="px-3 py-1.5 text-xs rounded-full text-ink-3 hover:text-ink">取消</button>
            <button @click="doDelete" class="btn-danger px-4 py-1.5 text-xs">删除</button>
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
