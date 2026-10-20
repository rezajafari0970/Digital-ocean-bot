# Datacenter create by api and ssh and install x-ui and create config

کنترل‌پنل ساخت سرور با API، نصب و مدیریت X-UI از طریق SSH و تولید کانفیگ‌های Direct و Residential. خروجی هر اکانت جداست.

## نصب خودکار

روی Ubuntu Server با کاربر **root** اجرا کنید (در صورت نیاز ابتدا `sudo -i`). این دستور حتی اگر curl نصب نباشد، پیش‌نیاز دریافت اسکریپت را هم نصب می‌کند:

```bash
apt-get update && apt-get install -y curl ca-certificates && curl -fsSL https://raw.githubusercontent.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/stable/install.sh -o /root/datacenter-install.sh && bash /root/datacenter-install.sh
```

نصب‌کننده بدون پرسش، پیش‌نیازها، PostgreSQL، Go با checksum ثابت، برنامه و سه سرویس systemd را نصب می‌کند. سرویس‌ها پس از ریبوت اجرا می‌شوند و در صورت خرابی دوباره راه‌اندازی می‌شوند. نصب اینترنت می‌خواهد؛ بستهٔ آفلاین نیست.

- سیستم‌های هدف: Ubuntu LTS 22.04، 24.04 و 26.04، معماری amd64 یا arm64.
- پیشنهاد منابع: حداقل 2 هسته، 2 گیگابایت RAM و 8 گیگابایت فضای خالی برای ساخت.
- پورت پیش‌فرض پنل: `18080`، مسیر `/admin`.
- نام کاربری و رمز اولیه: **admin / admin**.
- ورود اولیه مستقیم با **admin / admin** انجام می‌شود؛ لینک راه‌اندازی و تغییر اجباری رمز حذف شده‌اند. از **Panel Settings** می‌توانید نام کاربری و رمز را تغییر دهید. برای تغییر اطلاعات ورود، رمز فعلی لازم است؛ رمز جدید 12 تا 72 بایت است و خالی گذاشتن آن رمز فعلی را حفظ می‌کند.
- برای ورود امن از SSH tunnel یا HTTPS reverse proxy خودتان استفاده کنید. نمونهٔ تونل: `ssh -L 18080:127.0.0.1:18080 root@SERVER_IP`؛ سپس `http://127.0.0.1:18080/admin` را باز کنید. در صورت اتصال مستقیم، پورت را در فایروال میزبان و فایروال ارائه‌دهنده مجاز کنید.
- حساب ارائه‌دهنده، توکن‌ها، SSH و پروکسی‌های خود را پس از ورود در پنل اضافه کنید. هیچ حساب یا توکن عملیاتی در این مخزن وجود ندارد.
- برای کارکرد برنامه کلید OpenAI لازم نیست.

## تنظیمات پنل و آپدیت

در **Panel Settings** فقط پورت، مسیر وب، نام کاربری و رمز قابل تغییرند؛ آدرس گوش‌دادن همیشه `0.0.0.0` است. تغییر پورت و مسیر، API را خودکار راه‌اندازی مجدد می‌کند. در صورت تغییر پورت، آن را در فایروال سرور و ارائه‌دهنده هم مجاز کنید.

دکمهٔ **Update project** آخرین نسخهٔ تأییدشده را نصب می‌کند و وضعیت هفت مرحله را نمایش می‌دهد. اجرای آپدیت مستقل از API است و با بسته‌شدن صفحه یا ری‌استارت پنل قطع نمی‌شود. وضعیت پس از بازکردن دوبارهٔ تنظیمات قابل مشاهده است. دستور معادل:

```bash
sudo datacenter-bot update
```

نسخهٔ تأییدشده از شاخهٔ `stable` دریافت و قبل از توقف سرویس‌ها ساخته می‌شود. اطلاعات اکانت‌ها، رمز انتخابی، کلید رمزگذاری و تنظیمات حفظ می‌شوند. اجرای دوبارهٔ نصب نیز کاربران موجود را بازنشانی نمی‌کند. مهاجرت‌ها روی نصب موجود پیش از commit کنترل می‌شوند: حذف یا بازنویسی رکوردهای قبلی و حذف ستون‌ها آپدیت را متوقف می‌کند. تنها تبدیل یک‌بارهٔ وضعیت ورود اجباری و ثابت‌کردن host در مهاجرت 172 مجاز است؛ رمز کاربران تغییر نمی‌کند.

قبل از تغییر، پشتیبان دیتابیس، تنظیمات و پوشهٔ داده‌های پایدار در `/var/backups/datacenter-bot/` با دسترسی root ذخیره می‌شود. فایل‌های نسخهٔ قبل در `/opt/digital-ocean-bot.rollback.*` می‌مانند. شکست کنترل‌شدهٔ انتشار، بازگشت محافظت‌شده را اجرا می‌کند؛ اگر وضعیت دیتابیس بازگشت را ناامن کند، سرویس‌ها متوقف می‌مانند و پیام خطا مسیر بازیابی را نشان می‌دهد. پشتیبان دیتابیس به‌صورت خودکار روی داده‌های جدید بازنویسی نمی‌شود.

```bash
sudo datacenter-bot status
sudo datacenter-bot version
sudo datacenter-bot logs
sudo datacenter-bot recover
```

`recover` مخصوص انتشار قطع‌شده است؛ پس از بررسی پیام‌ها استفاده کنید. پشتیبان‌ها شامل اسرار هستند و نباید به GitHub فرستاده شوند. نگهداری و انتقال پشتیبان خارج از سرور بر عهدهٔ مالک نصب است.

## انتشار و همگام‌سازی

`main` و `stable` فقط با سورس بررسی‌شده به‌روز می‌شوند. سرویس انتشار روی سرور توسعه، تغییرات **commit‌شده** در پروژهٔ اصلی را بررسی می‌کند: درخت کد باید تمیز باشد، تست‌ها و اسکن اسرار باید موفق شوند و نسخهٔ مقصد تعارض نداشته باشد. فایل‌های موقت، دیتابیس، کلیدها، لاگ‌ها و تاریخچهٔ خصوصی منتقل نمی‌شوند. تغییر ثبت‌نشده منتشر نمی‌شود.

آپدیت GitHub خودکار است؛ نصب‌های کاربران با دکمهٔ پنل یا دستور `datacenter-bot update` ارتقا می‌یابند. هیچ کلید deploy یا توکن GitHub برای نصب کاربران لازم نیست.

## اصلاحات ساخت سرور و نمایش وضعیت

- نام هر سرور Linode از شناسهٔ پایدار استقرار ساخته می‌شود تا چند ساخت با یک پروفایل به نام تکراری برخورد نکنند. بازیابی ساخت‌های قبلی همچنان با برچسب مالکیت انجام می‌شود.
- Build Activity بدون پاک‌کردن جدول، جابه‌جایی اسکرول یا بستن جزئیات تازه می‌شود. شمارش فعال سراسری است؛ سوابق کامل‌شده جزو ساخت فعال نیستند.
- کل سرورهای ثبت‌شده جدا از سرورهای سالم نمایش داده می‌شود و علت افت سلامت قابل مشاهده است.
- تاریخچهٔ SSH و نصب از وضعیت فعلی تفکیک شده است؛ پیش‌بررسی عادی نصب خطای تکراری تولید نمی‌کند و خطاهای واقعی حفظ می‌شوند.

## ساخت از سورس

```bash
go test ./...
go vet ./...
python3 deploy/bootstrap_test.py
sudo bash install.sh --source "$(pwd)"
```

مسیر محلی باید سورس مورد اعتماد شما باشد. نام module برای سازگاری importهای پروژه حفظ شده است. قابلیت‌های اختیاری که به حساب ارائه‌دهنده و منابع واقعی نیاز دارند در تست نصب منابع خارجی ایجاد نمی‌کنند.

نتایج آزمون نسخه و ماتریس واقعی آزمون در `release/VALIDATION.json` ثبت می‌شود؛ امکان ساخت arm64 با آزمون واقعی روی ماشین arm64 یکسان نیست.

### Panel verification during installation

The installer checks the configured panel page and all referenced JavaScript/CSS against the release before reporting success. Static directories and files are readable by the service user even when downloaded with restrictive root permissions. Updates repair the earlier `403 Forbidden` issue while preserving existing configuration and login credentials.

### Linode expiry recovery

An expired managed Linode can retire below the configured Desired count when another server in the same account has fresh, verified service readiness. Retirement is serialized, revalidated before starting, and processed by the existing deletion journal. The next retirement waits for verified backfill recovery. Desired remains a growth target, not an asserted provider quota; normal growth spacing and provider/proxy admission remain unchanged.


## دقت وضعیت و خروجی موبایل

شمارنده‌های موجودی از همان دادهٔ ارائه‌دهنده در جدول محاسبه می‌شوند؛ نبود داده با موجودی صفر متفاوت است. زمان مشاهده و نوبت به‌روزرسانی مشخص‌اند و بازکردن صفحهٔ اکانت درخواست تازه‌سازی ناخواسته ایجاد نمی‌کند.

خطای تاریخی ساخت یا نصب از وضعیت فعلی جداست. تنظیمات پنل نسخهٔ واقعاً در حال اجرا را جدا از نتیجهٔ آپدیت قبلی نشان می‌دهد. گزینهٔ بررسی دوبارهٔ ظرفیت، دامنه‌های واجد شرایط را با تأیید صریح بررسی می‌کند و کلیک تکراری حین درخواست مهار می‌شود.

نمایش انسانی Output وضعیت بارگذاری، خالی‌بودن و خطا را مشخص می‌کند و خطوط بلند روی موبایل شکسته می‌شوند. قالب لینک خام اشتراک و مرز اکانت و نوع خروجی حفظ شده‌اند.


## وضعیت دقیق حساب‌ها
قفل حساب در ارائه‌دهنده، مشکل موقت شبکه و ظرفیت پر جدا نمایش داده می‌شوند. جزئیات هر حساب شامل علت، زمان مشاهده و وضعیت مستقل اتصال است. پاسخ‌های قدیمی یا مربوط به اعتبارنامه و اتصال قبلی نمی‌توانند وضعیت جدید را بازنویسی کنند. بازیابی پروکسی به‌تنهایی قفل ارائه‌دهنده را برطرف‌شده اعلام نمی‌کند؛ رفع محدودیت خارجی باید در پنل همان ارائه‌دهنده تأیید شود.


## تشخیص خطاهای ساخت و نصب
Build Activity خطای جاری، اجرای بازیابی‌شده و سابقهٔ سرور حذف‌شده را جدا نمایش می‌دهد. متن کامل خطا و خروجی در موبایل قابل مشاهده است. تحویل عادی به نصب‌کنندهٔ انتخاب‌شده خطای پیکربندی محسوب نمی‌شود. ادامهٔ نصب و اعلام تکمیل به پیش‌بررسی معتبر و ثبت پایدار نتیجه وابسته‌اند؛ فرمان با نتیجهٔ نامعلوم تنها از مسیر بازیابی محدود و قابل بررسی ادامه می‌یابد.

پیش‌بررسی‌های قدیمیِ نصب Sanaei نیز با وضعیت سرویس هنوز نصب‌نشده سازگارند؛ این سازگاری فقط همان فرمان‌های شناخته‌شده را پوشش می‌دهد و خطای واقعی فرمان را مجوز ادامهٔ نصب نمی‌داند.


## Additional IP inventory
The Additional IPs page reads official IP inventory per account for DigitalOcean, Vultr, Linode and UpCloud. Unknown quotas remain unknown; the last valid capacity and its provenance remain visible after failed reads. Panel and CLI reads share durable admission and provider cooldowns, including late rate-limit replies.
Allocation, attachment, rotation, release, guest-network changes and config publication using additional IPs are unavailable in this release. Discovered resources remain protected. Operational settings and canary cost/impact approval are still required. Reading IPv6 inventory does not change the existing IPv4 execution policy. The planning library is not an execution or availability guarantee.

### XHTTP Reality creation defaults
The Configs page can select TCP or XHTTP Reality stream-up for new physical listeners. XHTTP uses apart.com Host and a saved unique path of256–4096bytes. Direct and Residential clients on one physical listener share its path. Existing listeners retain their transport and credentials.
The exact requested FinalMask preset is visible but disabled because it is incompatible with the tested core. This profile uses native dialing, disabled WebView and empty Flow. Changing the creation default does not convert active listeners.

### Country names in exported configurations

VLESS names show the server's country flag and English name, such as 🇩🇪 Germany, in every global and account subscription. The country comes from provider region metadata, using the observed server region before the requested creation region. Names are resolved locally without a GeoIP request on each download. Existing connections, transport settings and native managed listener names are unchanged.

All 249 ISO country/territory codes are supported, plus the explicit Kosovo flag extension. Unrecognized metadata shows 🌐 Unknown country rather than an invented location. Refresh your subscription to receive the new display names; previously saved standalone links need to be imported again.


## دامنه و گواهی خودکار Cloudflare

در **Configurations → Cloudflare domains** توکن API یا Global API key و ایمیل را ثبت کنید، سپس دامنه‌های فعال را انتخاب کنید. توکن محدود به دامنه‌های موردنیاز پیشنهاد می‌شود. مجوزهای Zone Read، DNS Edit، SSL and Certificates Edit و Zone Settings Read لازم‌اند؛ برای تغییر خودکار Full (strict)، مجوز Zone Settings Edit هم لازم است. گزینهٔ Full (strict) روی کل دامنه اعمال می‌شود.

در پروفایل **WS + TLS**، گزینهٔ استفادهٔ خودکار از دامنه‌های Cloudflare را فعال کنید. درایور اولیه از VLESS، VMess و Trojan روی WS پشتیبانی می‌کند؛ TCP خام، REALITY، gRPC و XHTTP در این قابلیت فعال نمی‌شوند. پورت خودکار از پورت‌های HTTPS مجاز و آزاد انتخاب می‌شود.

پیش‌فرض هر سرور یک رکورد در هر ۳۰ دقیقه، عمر مستقل تصادفی ۴۵ تا ۶۰ دقیقه و سقف چهار رکورد هم‌زمان است. تعداد و بازه قابل تنظیم‌اند؛ سقف نسخهٔ فعلی دو رکورد در هر نوبت و هشت رکورد فعال در هر پروفایل/سرور است. گواهی Origin CA روی سرور با کلید خصوصی محلی نصب و خودکار تمدید می‌شود. کلید Cloudflare فقط در کنترلر رمزگذاری می‌شود. خروجی تنها پس از آزمون واقعی مسیر CDN و بررسی کامل TLS منتشر می‌شود.

برای IP اپراتور، منبع رسمی محدوده‌های Cloudflare استفاده می‌شود. محدودهٔ رسمی معادل IP آزمایش‌شده در ایران نیست. از رکورد آماده **Download probe target** بگیرید و با Xray رسمی روی شبکهٔ همان اپراتور اجرا کنید:

```bash
python3 tools/cloudflare/operator-probe.py --target-file cloudflare-probe-ID.json --operator mci --xray /path/to/xray > operator-results.json
```

نتیجه را از **Import operator probe results** وارد کنید. بدون نتیجهٔ تازه، دامنهٔ خودتان استفاده می‌شود. فایل هدف شامل کانفیگ کلاینت است؛ توکن Cloudflare به گوشی منتقل نمی‌شود. برای Irancell مقدار `irancell` و برای Rightel مقدار `rightel` را انتخاب کنید.

لینک ثابت با حذف دامنهٔ قدیمی منقضی می‌شود؛ اشتراک باید به‌موقع تازه شود. TTL و کش DNS جدا از عمر رکورد هستند و قطعی API ممکن است حذف واقعی را عقب بیندازد. قابلیت برای کانفیگ‌های موجود خاموش است و قوانین خروجی Google Ads/Residential را تغییر نمی‌دهد. پذیرش زنده به حساب و دامنهٔ خودتان و اندازه‌گیری روی شبکهٔ واقعی اپراتور نیاز دارد؛ نرخ بارگذاری ۹۹٪ یا کارکرد بدون خطا تضمین نمی‌شود.
