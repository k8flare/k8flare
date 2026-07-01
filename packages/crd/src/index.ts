export { handleResourceCRUD } from "./crud.ts";
export { applyCommonDefaults } from "./defaults.ts";
export {
  buildDiscoveryResources,
  buildDiscoveryGroup,
  injectCustomAPIGroup,
} from "./discovery.ts";
export { parseCustomGroupPath } from "./path-parser.ts";
export {
  dwStub,
  crKey,
  crListPrefix,
  crGet,
  crList,
  crPut,
  crDelete,
} from "./storage.ts";
export type {
  ObjectMeta,
  CRDConfig,
  CustomResource,
  APIResourceDescriptor,
  ParsedPath,
  CRGetResult,
} from "./types.ts";
