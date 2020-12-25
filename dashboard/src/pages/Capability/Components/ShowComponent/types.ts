export interface ShowParameters {
  name: string;
  parameters: Parameters[];
}

interface Parameters {
  name: string;
  usage: string;
  default: string;
  required: boolean;
  short: string;
}
