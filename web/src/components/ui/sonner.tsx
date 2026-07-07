import * as React from "react";
import { Toaster as SonnerToaster, toast } from "sonner";

type ToasterProps = React.ComponentProps<typeof SonnerToaster>;

/**
 * Reads the active theme from the root <html> class (kept in sync by the
 * theme toggle in App.tsx) so toasts match light/dark without next-themes.
 * Kapibara is dark-first: the `.light` class opts into the light palette.
 */
function useDocumentTheme(): "light" | "dark" {
  const [theme, setTheme] = React.useState<"light" | "dark">(() =>
    typeof document !== "undefined" && document.documentElement.classList.contains("light")
      ? "light"
      : "dark"
  );
  React.useEffect(() => {
    const el = document.documentElement;
    const sync = () => setTheme(el.classList.contains("light") ? "light" : "dark");
    sync();
    const observer = new MutationObserver(sync);
    observer.observe(el, { attributes: true, attributeFilter: ["class"] });
    return () => observer.disconnect();
  }, []);
  return theme;
}

export function Toaster(props: ToasterProps) {
  const theme = useDocumentTheme();
  return (
    <SonnerToaster
      theme={theme}
      position="bottom-right"
      toastOptions={{
        classNames: {
          toast:
            "group toast group-[.toaster]:bg-card group-[.toaster]:text-foreground group-[.toaster]:border group-[.toaster]:border-border group-[.toaster]:rounded-lg group-[.toaster]:shadow-lg",
          description: "group-[.toast]:text-muted-foreground",
          actionButton: "group-[.toast]:bg-primary group-[.toast]:text-primary-foreground",
          cancelButton: "group-[.toast]:bg-secondary group-[.toast]:text-secondary-foreground",
          error: "group-[.toaster]:!text-destructive",
          success: "group-[.toaster]:!text-success",
        },
      }}
      {...props}
    />
  );
}

export { toast };
