import './style.css'
import { createApp, nextTick } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import Home from './views/Home.vue'
import AlbumList from './views/AlbumList.vue'
import AlbumDetail from './views/AlbumDetail.vue'
import Search from './views/Search.vue'
import Login from './views/Login.vue'
import InviteRedeem from './views/InviteRedeem.vue'
import AdminInvites from './views/AdminInvites.vue'
import AdminLogs from './views/AdminLogs.vue'
import AdminHome from './views/admin/AdminHome.vue'
import AdminStatus from './views/admin/AdminStatus.vue'
import UploadPage from './views/UploadPage.vue'
import { authState, checkAuth, isAdmin } from './stores/auth'
import { scrollHostToTop } from './lib/scrollHost'

// `chromeless` routes render without the app shell (no sidebar, no tab bar).
const routes = [
  { path: '/login', component: Login, meta: { noAuth: true, chromeless: true } },
  { path: '/invite/:code', component: InviteRedeem, meta: { noAuth: true, chromeless: true } },
  { path: '/', component: Home },
  { path: '/albums', component: AlbumList },
  { path: '/albums/:id', component: AlbumDetail },
  { path: '/search', component: Search },
  // Admin-only pages. The APIs behind them carry AdminOnly() as well; this
  // guard is what keeps the pages themselves unreachable for guests.
  { path: '/upload', component: UploadPage, meta: { adminOnly: true } },
  { path: '/admin', component: AdminHome, meta: { adminOnly: true } },
  { path: '/admin/status', component: AdminStatus, meta: { adminOnly: true } },
  { path: '/admin/invites', component: AdminInvites, meta: { adminOnly: true } },
  { path: '/admin/logs', component: AdminLogs, meta: { adminOnly: true } },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach(async (to, _from, next) => {
  if (!authState.checked) {
    await checkAuth()
  }
  if (to.meta.noAuth) {
    next()
  } else if (!authState.user) {
    next('/login')
  } else if (to.meta.adminOnly && !isAdmin.value) {
    next('/')
  } else {
    next()
  }
})

// Every navigation starts at the top of the current scroll host. Neither the
// document nor the inner <main> is reset automatically by the router.
router.afterEach(() => {
  nextTick(() => scrollHostToTop(false))
})

createApp(App).use(router).mount('#app')
