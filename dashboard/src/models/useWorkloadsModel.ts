import { useEffect, useState } from 'react';

import * as api from '@/services/capability';

interface State {
  loading?: boolean;
  workloadList?: API.Workloads[] | null;
}

export default function useWorkloadsModel() {
  const [state, setState] = useState<State>({});

  const setWorkloads = (workloadList: API.Workloads[]) => {
    setState({ loading: false, workloadList });
  };

  const getWorkloadsList = async () => {
    setState({ ...state, loading: true });
    const { data } = await api.getWorkloads();
    setWorkloads(data);
    return data;
  };

  useEffect(() => {
    getWorkloadsList();
  }, []);

  return {
    ...state,
    getWorkloadsList,
  };
}
