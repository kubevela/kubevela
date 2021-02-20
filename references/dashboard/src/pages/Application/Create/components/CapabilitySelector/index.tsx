import { Select, Typography } from 'antd';
import { useModel } from 'umi';

export interface CapabilitySelectorProps {
  capability: 'service' | 'trait';
  onSelect?: (name: string) => void;
  disableCapabilities?: string[];
}
const CapabilitySelector: React.FC<CapabilitySelectorProps> = ({
  capability,
  onSelect,
  disableCapabilities,
}) => {
  const workloadModel = useModel('useWorkloadsModel');
  const traitModel = useModel('useTraitsModel');

  let loading: boolean | undefined;
  let data: { name: string; description?: string }[] | null;

  switch (capability) {
    case 'service':
      loading = workloadModel.loading;
      data = workloadModel.workloadList ?? null;
      break;
    case 'trait':
      loading = traitModel.loading;
      data = traitModel.traitsList ?? null;
      break;
    default:
      throw new Error(`invalid capability: '${capability}'.`);
  }

  return (
    <Select
      onSelect={onSelect == null ? undefined : (e) => onSelect(e.toString())}
      loading={loading}
      placeholder={`Select a ${capability}`}
      style={{ width: '100%' }}
      optionLabelProp="value"
    >
      {data?.map((item) => (
        <Select.Option
          key={item.name}
          value={item.name}
          disabled={disableCapabilities == null ? false : disableCapabilities.includes(item.name)}
        >
          {item.name}
          <Typography.Text type="secondary" style={{ fontSize: '10px', marginLeft: '5px' }}>
            {item.description}
          </Typography.Text>
        </Select.Option>
      ))}
    </Select>
  );
};
export default CapabilitySelector;
