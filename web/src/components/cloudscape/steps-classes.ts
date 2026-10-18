// Official Cloudscape class names for the Steps component
// (from @cloudscape-design/components@3.0.1401 steps/styles.css.js).
// Kept beside the ported component instead of the generated classes.ts,
// which only carries the components already in use when it was generated.
export const STEPS = {
  root: 'awsui_root_gxp9y_o8puq_21',
  list: 'awsui_list_gxp9y_o8puq_54',
  container: 'awsui_container_gxp9y_o8puq_59',
  annotation: 'awsui_annotation_gxp9y_o8puq_64',
  header: 'awsui_header_gxp9y_o8puq_17',
  'step-layout': 'awsui_step-layout_gxp9y_o8puq_69',
  details: 'awsui_details_gxp9y_o8puq_75',
  horizontal: 'awsui_horizontal_gxp9y_o8puq_85',
  content: 'awsui_content_gxp9y_o8puq_121',
  vertical: 'awsui_vertical_gxp9y_o8puq_135',
  'connector-continuation': 'awsui_connector-continuation_gxp9y_o8puq_149',
  connector: 'awsui_connector_gxp9y_o8puq_9',
  'connector-hidden': 'awsui_connector-hidden_gxp9y_o8puq_13',
  'with-annotation': 'awsui_with-annotation_gxp9y_o8puq_190',
} as const

// Official status-text tokens, used for the step header colour so the text
// stays readable in dark mode (hard-coded hex would not).
export const STEP_STATUS_COLOR: Record<string, string> = {
  error: 'var(--color-text-status-error-ri38gt, #d13212)',
  warning: 'var(--color-text-status-warning-fabq6h, #8d6605)',
  success: 'var(--color-text-status-success-1i2jf8, #037f0c)',
  info: 'var(--color-text-status-info-su6k9n, #006ce0)',
  stopped: 'var(--color-text-status-inactive-fqjc97, #656871)',
  pending: 'var(--color-text-status-inactive-fqjc97, #656871)',
  'in-progress': 'var(--color-text-status-inactive-fqjc97, #656871)',
  loading: 'var(--color-text-status-inactive-fqjc97, #656871)',
  'not-started': 'var(--color-text-status-inactive-fqjc97, #656871)',
  log: 'var(--color-text-status-inactive-fqjc97, #656871)',
}

export const DIVIDER = {
  divider: 'awsui_divider_1noo2_gvayg_9',
  horizontal: 'awsui_divider-horizontal_1noo2_gvayg_45',
  vertical: 'awsui_divider-vertical_1noo2_gvayg_74',
  'has-label': 'awsui_divider-has-label_1noo2_gvayg_52',
  line: 'awsui_divider-line_1noo2_gvayg_61',
  label: 'awsui_divider-label_1noo2_gvayg_68',
} as const
