import './style.css'
import { createApp } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import Home from './views/Home.vue'
import AlbumList from './views/AlbumList.vue'
import AlbumDetail from './views/AlbumDetail.vue'

const routes = [
  { path: '/', component: Home },
  { path: '/albums', component: AlbumList },
  { path: '/albums/:id', component: AlbumDetail },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

createApp(App).use(router).mount('#app')
