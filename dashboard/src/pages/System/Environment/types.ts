export type EditMode = 'create' | 'update';
export interface EditorState {
  mode?: EditMode;
  environment?: Environment | null;
}

export interface Environment {
  envName: string;
  namespace: string;
}
