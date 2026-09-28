import { component$ } from "@builder.io/qwik";
import { useDocumentHead } from "@builder.io/qwik-city";

export const RouterHead = component$(() => {
  const head = useDocumentHead();
  const base = import.meta.env.BASE_URL;

  return (
    <>
      <title>{head.title}</title>
      <meta name="viewport" content="width=device-width, initial-scale=1.0" />
      <meta name="theme-color" content="#0F172A" />
      <meta name="robots" content="noindex" />
      <link rel="icon" type="image/svg+xml" href={`${base}favicon.svg`} />
      {head.meta.map((m) => (
        <meta key={m.key} {...m} />
      ))}
      {head.links.map((l) => (
        <link key={l.key} {...l} />
      ))}
    </>
  );
});
