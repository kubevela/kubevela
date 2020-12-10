import { useEffect, useState } from 'react';

import * as api from '@/services/environment';

interface State {
  loading?: boolean;
  environments?: API.Environment[] | null;
  currentEnvironment?: API.Environment | null;
}

const DEFAULT_ENVIRONMENT_NAME = 'default';

export default function useEnvironmentModel() {
  const [state, setState] = useState<State>({});

  const setEnvironments = (environments: API.Environment[]) => {
    const currentEnvironment = environments.find(
      ({ current }) => current != null && current !== '',
    );
    setState({ loading: false, environments, currentEnvironment });
  };

  const getEnvironments = async () => {
    setState({ ...state, loading: true });
    const { data } = await api.getEnvironments();
    setEnvironments(data);
    return data;
  };

  useEffect(() => {
    getEnvironments();
  }, []);

  return {
    ...state,
    getEnvironments,
    switchCurrentEnvironment: async (name: string) => {
      await api.switchCurrentEnvironment(name);
      const environments = await getEnvironments();
      return environments.find((e) => e.envName === name);
    },
    deleteEnvironment: async (name: string) => {
      if (state.currentEnvironment?.envName === name) {
        await api.switchCurrentEnvironment(DEFAULT_ENVIRONMENT_NAME);
      }
      const response = await api.deleteEnvironment(name);
      getEnvironments();
      return response;
    },
    createEnvironment: async (environment: API.Environment) => {
      const response = await api.createEnvironment(environment);
      getEnvironments();
      return response;
    },
    updateEnvironment: async (name: string, environment: API.EnvironmentBody) => {
      const response = await api.updateEnvironment(name, environment);
      getEnvironments();
      return response;
    },
  };
}
