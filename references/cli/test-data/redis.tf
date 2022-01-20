terraform {
  required_providers {
    tencentcloud = {
      source = "tencentcloudstack/tencentcloud"
    }
  }
}

resource "tencentcloud_redis_instance" "main" {
  type_id           = 8
  availability_zone = var.availability_zone
  name              = var.instance_name
  password          = var.user_password
  mem_size          = var.mem_size
  port              = var.port
}

output "DB_IP" {
  value = tencentcloud_redis_instance.main.ip
}

output "DB_PASSWORD" {
  value = var.user_password
}

output "DB_PORT" {
  value = var.port
}

variable "availability_zone" {
  description = "The available zone ID of an instance to be created."
  type        = string
  default = "ap-chengdu-1"
}

variable "instance_name" {
  description = "redis instance name"
  type        = string
  default     = "sample"
}

variable "user_password" {
  description = "redis instance password"
  type        = string
  default     = "IEfewjf2342rfwfwYYfaked"
}

variable "mem_size" {
  description = "redis instance memory size"
  type        = number
  default     = 1024
}

variable "port" {
  description = "The port used to access a redis instance."
  type        = number
  default     = 6379
}
