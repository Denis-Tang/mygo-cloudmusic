// The website uses the same gold logo as the desktop app.
export function AppIcon(props: { size?: number; class?: string }) {
  return (
    <img
      src="/favicon.svg"
      class={props.class}
      width={props.size ?? 32}
      height={props.size ?? 32}
      alt=""
      aria-hidden="true"
    />
  )
}
