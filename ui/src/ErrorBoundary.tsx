import React from "react";
export class ErrorBoundary extends React.Component<{children: React.ReactNode}, {error: any}> {
  state = { error: null as any };
  static getDerivedStateFromError(error: any) { return { error }; }
  render() {
    if (this.state.error) return <pre style={{padding:16,color:'red',whiteSpace:'pre-wrap'}}>
      {String(this.state.error?.message || this.state.error)}
    </pre>;
    return this.props.children as any;
  }
}
