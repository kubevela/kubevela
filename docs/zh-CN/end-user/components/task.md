---
title:  Task
---

## 定义

描述运行代码或脚本以完成的作业。

## 示例

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-worker
spec:
  components:
    - name: mytask
      type: task
      properties:
        image: perl
	    count: 10
	    cmd: ["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"]
```

## 属性说明

```console
# Properties
+---------+--------------------------------------------------------------------------------------------------+----------+----------+---------+
|  NAME   |                                           DESCRIPTION                                            |   TYPE   | REQUIRED | DEFAULT |
+---------+--------------------------------------------------------------------------------------------------+----------+----------+---------+
| cmd     | Commands to run in the container                                                                 | []string | false    |         |
| count   | specify number of tasks to run in parallel                                                       | int      | true     |       1 |
| restart | Define the job restart policy, the value can only be Never or OnFailure. By default, it's Never. | string   | true     | Never   |
| image   | Which image would you like to use for your service                                               | string   | true     |         |
+---------+--------------------------------------------------------------------------------------------------+----------+----------+---------+
```
