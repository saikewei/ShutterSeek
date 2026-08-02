import './style.css'
import { createApp } from 'vue'
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
import { authState, checkAuth } from './stores/auth'

const routes = [
  { path: '/login', component: Login, meta: { noAuth: true, hideSidebar: true } },
  { path: '/invite/:code', component: InviteRedeem, meta: { noAuth: true, hideSidebar: true } },
  { path: '/', component: Home },
  { path: '/albums', component: AlbumList },
  { path: '/albums/:id', component: AlbumDetail },
  { path: '/search', component: Search },
  { path: '/admin/invites', component: AdminInvites },
  { path: '/admin/logs', component: AdminLogs },
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
  } else {
    next()
  }
})

createApp(App).use(router).mount('#app')
