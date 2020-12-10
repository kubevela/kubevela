declare namespace API {
  export interface VelaResponse<T> {
    code: number;
    data: T;
  }

  export interface ApplicationMeta {
    name: string;
    status?: string;
    components?: ComponentMeta[];
    createdTime?: Date;
  }

  export interface ComponentMeta {
    name: string;
    status?: string;
    workload?: any;
    workloadName?: string;
    traits?: any[]; // ComponentTrait
    traitsNames?: string[];
    app: string;
    createdTime?: Date;
  }

  interface Environment {
    envName: string;
    namespace: string;
    email: string;
    domain: string;
    current?: string;
  }

  interface EnvironmentBody {
    namespace: string;
  }
}
