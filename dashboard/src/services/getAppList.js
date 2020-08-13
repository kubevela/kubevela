import request from '@/utils/request' ;
export async function doit (payload) {
  return request('/love/record')
}