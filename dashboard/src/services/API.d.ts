declare namespace API {
  export interface VelaResponse<T> {
    code: number;
    data: T;
  }

  export interface ApplicationMeta {
    name: string;
    status?: string;
    components?: ComponentMeta[];
    createdTime?: string;
  }

  export interface ComponentMeta {
    name: string;
    status?: string;
    workload?: any;
    workloadName?: string;
    traits?: any[]; // ComponentTrait
    traitsNames?: string[];
    app: string;
    createdTime?: string;
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

  interface Application {
    name: string;
    status: string;
    createdTime: string;
  }

  interface Workloads {
    name: string;
    parameters: Parameters[];
  }

  interface Parameters {
    name: string;
    short: string;
    usage: string;
    default: string;
    required: boolean;
  }

}
