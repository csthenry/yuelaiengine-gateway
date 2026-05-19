import axios from 'axios'
import { getStoredToken } from '../stores/auth'

const http = axios.create({
  timeout: 10000
})

http.interceptors.request.use((config) => {
  const token = getStoredToken()
  if (token) {
    config.headers['X-Admin-Token'] = token
  }
  return config
})

http.interceptors.response.use(
  (response) => response,
  (error) => {
    if (!error.response) {
      return Promise.reject({
        status: 0,
        code: 'NETWORK_ERROR',
        message: '网络连接失败或网关未启动'
      })
    }

    const { status, data } = error.response
    return Promise.reject({
      status,
      code: data?.code || data?.status || 'REQUEST_FAILED',
      message: data?.message || `请求失败 (${status})`,
      raw: data
    })
  }
)

export default http
