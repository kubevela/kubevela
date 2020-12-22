export interface Workloads {
  name: string;
  parameters: Parameters[];
}

interface Parameters {
  name: string;
  usage: string;
  default: string;
  required: boolean
  short: string;
}
