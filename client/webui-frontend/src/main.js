import { createApp } from 'vue'
import naive from 'naive-ui'
import App from './App.vue'
import router from './router.js'
import 'vfonts/Lato.css'
import 'vfonts/FiraCode.css'
import './assets/style.css'

const app = createApp(App)
app.use(router)
app.use(naive)
app.mount('#app')
