There is no server-side `t()` in Go — translated labels are rendered in the **browser** by the client SDK once `I18nScriptTag` has loaded the `{{PROFILE}}` profile.

```js
// Browser (client SDK), after the loader installs the {{PROFILE}} labels:
const label = shipeasy.t("checkout.cta");
```
