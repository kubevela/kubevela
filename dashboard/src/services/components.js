import request from '@/utils/request';
/*
 * GET /api/envs/:envName/apps/:appName/components/ (component list)
 * Same as GET /api/envs/:envName/apps/:appName (app description).
 */
export async function getComponentList({ envName, appName }) {
  return request(`/api/envs/${envName}/apps/${appName}/components/`);
}
/*
 * GET /api/envs/:envName/apps/:appName/components/:compName (component details)
 */
export async function getComponentDetail({ envName, appName, compName }) {
  return request(`/api/envs/${envName}/apps/${appName}/components/${compName}`);
}
/*
 * DELETE /api/envs/:envName/apps/:appName/components/:compName (component delete)
 */
export async function deleteComponent({ envName, appName, compName }) {
  return request(`/api/envs/${envName}/apps/${appName}/components/${compName}`, {
    method: 'delete',
  });
}
