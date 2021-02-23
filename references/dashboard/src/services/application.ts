import { request } from 'umi';

const BASE_PATH = '/api/envs';

/*
 * application list: get /api/envs/{env}/apps
 */
export async function getApplications(
  envName: string,
): Promise<API.VelaResponse<API.Application[]>> {
  return request(`${BASE_PATH}/${envName}/apps`);
}

/*
 * delete application: delete /api/envs/{env}/apps/{app}
 */
export async function deleteApplication(
  envName: string,
  appName: string,
): Promise<API.VelaResponse<string>> {
  return request(`${BASE_PATH}/${envName}/apps/${appName}`, { method: 'delete' });
}

// create an application
export async function createApplication(
  envName: string,
  appFile: API.AppFile,
): Promise<API.VelaResponse<API.Application[]>> {
  return request(`${BASE_PATH}/${envName}/apps`, {
    method: 'post',
    data: appFile,
  });
}
