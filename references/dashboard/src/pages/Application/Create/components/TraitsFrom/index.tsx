import React, { useState } from 'react';

import { Tabs } from 'antd';

import CapabilityFormItem from '../CapabilityFormItem';

interface TraitItem {
  id: number;
  type?: string;
}
const TraitsFrom: React.FC<{
  onChange?: (data: { [key: string]: object }) => void;
  onValidate?: (errorFields: { [field: string]: any }) => void;
}> = ({ onChange, onValidate }) => {
  const [items, setItems] = useState<TraitItem[]>([{ id: 1 }]);
  const [activeId, setActiveId] = useState<number>(1);
  const [data, setData] = useState<{ [key: string]: object }>({});

  const removeFormData = (key: string) => {
    delete data[key];
  };

  const addItem = () => {
    const newId = items.length + 1;
    setItems([...items, { id: newId }]);
    setActiveId(newId);
  };

  const removeItem = (id: number) => {
    const removedItem = items.find((i) => i.id === id);
    if (removedItem == null) {
      return;
    }
    if (removedItem.type != null) {
      removeFormData(removedItem.type);
    }

    setData({ ...data });
    const newItems = items.filter((i) => i !== removedItem);
    setItems(newItems);
    const { length } = newItems;
    if (length > 0) {
      setActiveId(newItems[length - 1].id);
    }
  };

  const updateItem = (id: number, updater: (item: TraitItem) => TraitItem) => {
    const index = items.findIndex((i) => i.id === id);
    if (index === -1) {
      return;
    }
    const current = items[index];
    items[index] = updater(current);
    setItems([...items]);
  };

  return (
    <div>
      <Tabs
        type="editable-card"
        tabPosition="top"
        activeKey={activeId.toString()}
        onChange={(e) => setActiveId(parseFloat(e))}
        onEdit={(key, action) => {
          switch (action) {
            case 'add':
              addItem();
              break;
            case 'remove':
              removeItem(parseFloat(key.toString()));
              break;
            default:
              throw new Error(`invalid action '${action}'.`);
          }
        }}
      >
        {items.map((item) => (
          <Tabs.TabPane key={item.id} tab={item.type ?? 'New trait'} closable>
            <CapabilityFormItem
              capability="trait"
              onSelect={(e) => {
                updateItem(item.id, (i) => ({ ...i, type: e }));
              }}
              onChange={(current, old) => {
                if (old?.capabilityType != null) {
                  removeFormData(old.capabilityType);
                }
                data[current.capabilityType] = current.data;
                const newData = { ...data };
                setData(newData);
                if (onChange != null) {
                  onChange(newData);
                }
              }}
              disableCapabilities={Object.keys(data)}
              onValidate={
                onValidate == null
                  ? undefined
                  : (fields) => {
                      onValidate(Object.keys(fields).length === 0 ? {} : { traits: fields });
                    }
              }
            />
          </Tabs.TabPane>
        ))}
      </Tabs>
    </div>
  );
};

export default TraitsFrom;
