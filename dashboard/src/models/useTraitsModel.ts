import { useEffect, useState } from 'react';

import * as api from '@/services/traits';

interface State {
  loading?: boolean;
  traitsList?: API.Traits[] | null;
}

export default function useTraitsModel() {
  const [state, setState] = useState<State>({});

  const setTraits = (traitsList: API.Traits[]) => {
    setState({ loading: false, traitsList });
  };

  const getTraitsListList = async () => {
    setState({ ...state, loading: true });
    const { data } = await api.getTraits();
    setTraits(data);
    return data;
  };

  useEffect(() => {
    getTraitsListList();
  }, []);

  return {
    ...state,
    getTraitsListList,
  };
}
