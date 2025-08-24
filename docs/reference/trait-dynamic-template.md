# TraitDefinition Dynamic Template Guide

This guide explains how to use KubeVela's dynamic template mode for TraitDefinitions, which allows trait templates to be rendered via API calls rather than static templates.

## Overview

TraitDefinitions can use two template modes:
1. **Static Mode**: Traditional CUE template defined in the TraitDefinition
2. **Dynamic Mode**: Template rendered via API calls to external services

Dynamic template mode is useful when:
- You need complex logic that's difficult to express in CUE
- You want to integrate with external systems or databases
- You need to query cloud resources (e.g., RDS connection strings)
- You want to implement custom security or compliance checks

## How Dynamic Template Mode Works

When dynamic template mode is enabled:
1. KubeVela calls your API endpoint with the trait parameters and workload reference
2. Your API returns a template patch that will be applied to the base resource
3. KubeVela applies this patch to create the final resource

## Configuring Dynamic Template Mode

To enable dynamic template mode, set the following fields in your TraitDefinition:
