import request from '@/utils/request';
/*
 * 创建 workload：POST /api/workloads/
 */
export async function createWorkload({ params }) {
  return request('/api/workloads/', {
    method: 'post',
    data: {
      ...params,
    },
    headers: {
      'Content-Type': 'application/json',
    },
  });
}
/*
 * workloads 列表 GET /api/workloads/
 */
export async function getWorkload() {
  return request('/api/workloads/');
}
/*
 * GET /api/workloads/:workloadName
 */
export async function getWorkloadByName({ workloadName }) {
  return request(`/api/workloads/${workloadName}`);
}
