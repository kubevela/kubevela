import request from '@/utils/request';
/*
 * Get /capability-centers/ (Capability Center 列表)
 */
export async function getCapabilityCenterlist() {
  return request('/api/capability-centers/');
}
/*
 * Put /capability-centers/ (添加 Capability Center)
 */
export async function createCapability({ params }) {
  return request('/api/capability-centers/', {
    method: 'put',
    // body:JSON.stringify(params),
    data: {
      ...params,
    },
    headers: {
      'Content-Type': 'application/json',
    },
  });
}
/*
 * Put /capability-centers/:capabilityCenterName/capabilities/  (sync Capability Center)
 */
export async function syncCapability({ capabilityCenterName }) {
  return request(`/api/capability-centers/${capabilityCenterName}/capabilities/`, {
    method: 'put',
    headers: {
      'Content-Type': 'application/json',
    },
  });
}
/*
 * Put /capability-centers/:capabilityCenterName/capabilities/:capabilityName  (安装一个 capability)
 */
export async function syncOneCapability({ capabilityCenterName, capabilityName }) {
  return request(`/api/capability-centers/${capabilityCenterName}/capabilities/${capabilityName}`, {
    method: 'put',
    headers: {
      'Content-Type': 'application/json',
    },
  });
}
/*
 * Delete /api/capabilities/:capabilityName (删除一个 capability)
 */
export async function deleteOneCapability({ capabilityName }) {
  return request(`/api/capabilities/${capabilityName}`, {
    method: 'delete',
  });
}
/*
 * Get /api/capabilities/ (capability 列表)
 */
export async function capabilityList() {
  return request(`/api/capabilities/`);
}
