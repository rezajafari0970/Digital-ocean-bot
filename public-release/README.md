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

## Amazon AWS EC2 و OVH Public Cloud

هر دو ارائه‌دهنده از Registry و درایور مستقل استفاده می‌کنند و در Accounts قابل انتخاب‌اند. فرم اعتبارنامه از متادیتای همان درایور ساخته می‌شود. کاتالوگ، موجودی، ساخت، SSH، نصب پنل، انقضا، حذف و بازیابی از مسیر مشترک چرخهٔ عمر اجرا می‌شوند. اعتبارنامه‌ها در مخزن اسرار موجود رمزگذاری می‌شوند.

### AWS

یک حساب می‌تواند یک منطقهٔ اصلی و حداکثر چهار منطقهٔ اضافه با شبکهٔ مستقل داشته باشد. مناطق اضافه را در بخش کشف شبکه انتخاب کنید و گزینه‌ها را دوباره بارگذاری کنید؛ فقط مناطق دارای شبکه، پلن و Ubuntu تأییدشده در انتخاب‌های ساخت ظاهر می‌شوند. حساب‌های تک‌منطقه‌ای موجود همان هویت و محدودهٔ قبلی را حفظ می‌کنند. فقط Access key ID و Secret access key را وارد کنید؛ فهرست مناطق فعال و Subnet عمومی به‌صورت خودکار خوانده می‌شود. گزینهٔ انتخاب‌شده پیش از ذخیره نمایش داده می‌شود و ورود دستی شناسه‌های شبکه لازم نیست. برای اعتبارنامهٔ موقت، Session token نیز وارد شود و پیش از انقضا با اعتبارنامهٔ همان حساب و منطقه جایگزین شود.

کشف خودکار، مسیر پیش‌فرض فعال به Internet Gateway متصل و قواعد IPv4 شبکه را بررسی می‌کند. با تأیید گزینهٔ شبکهٔ خودکار در فرم، هنگام اولین ساخت سرور یک Security group اختصاصی ساخته می‌شود: ورودی IPv4 TCP/UDP پورت‌های ۱ تا ۶۵۵۳۵ برای SSH و پورت‌های متغیر سرویس، با قواعد خروجی پیش‌فرض AWS. گروه‌های موجود تغییر نمی‌کنند. می‌توانید در Advanced overrides گروه‌های موجود خود را معرفی کنید؛ قواعد دسترسی آن‌ها به عهدهٔ شما است. برنامه VPC، subnet، gateway، NAT یا Elastic IP ایجاد نمی‌کند. اگر شبکهٔ عمومی مناسب وجود نداشته باشد، منطقهٔ دیگری انتخاب کنید یا شبکه را در AWS آماده کنید. شناسهٔ حساب از STS تأیید می‌شود و تغییر اعتبارنامه به حساب/منطقهٔ دیگری رد می‌شود.

مجوزهای EC2 موردنیاز: `DescribeInstances`، `DescribeImages`، `DescribeInstanceTypes`، `DescribeInstanceTypeOfferings`، `DescribeRegions`، `DescribeSubnets`، `DescribeRouteTables`، `DescribeNetworkAcls`، `DescribeInternetGateways`، `DescribeSecurityGroups`، `RunInstances`، `CreateTags` برای برچسب‌های زمان ساخت، و `TerminateInstances`. سیاست IAM باید منابع وابستهٔ مجاز RunInstances را هم پوشش دهد؛ محدودکردن TerminateInstances به برچسب مالکیت برنامه توصیه می‌شود. STS GetCallerIdentity برای اثبات هویت فراخوانی می‌شود. شبکهٔ خودکار همچنین به `CreateSecurityGroup`، `AuthorizeSecurityGroupIngress`، `DescribeNetworkInterfaces` و `DeleteSecurityGroup` نیاز دارد.

دامنهٔ فعلی: EC2 On-Demand در پارتیشن تجاری AWS، Ubuntu رسمی Canonical روی x86_64، دیسک ریشهٔ gp3 با حذف همراه سرور، IPv4 عمومی و IMDSv2 اجباری. ARM، GovCloud/China، Spot، دیسک اضافه و Elastic IP در این درایور عرضه نمی‌شوند. سقف تعداد سرورها از سهمیهٔ vCPU حدس زده نمی‌شود و قیمت نامعلوم صریحاً نامعلوم می‌ماند. هزینهٔ واقعی نمونه، دیسک، IPv4 و ترافیک تابع حساب AWS است.

قیمت محاسبات از AWS Price List Query API و با مجوز اختیاری `pricing:GetProducts` روی منبع `*` خوانده می‌شود. نرخ نمایش‌داده‌شده مخصوص Linux، Shared tenancy و On-Demand در منطقهٔ انتخاب‌شده است. دیسک EBS gp3، IPv4 عمومی، ترافیک، اعتبار CPU و مالیات جدا هستند؛ تخمین ماهانه برابر ۷۳۰ ساعت است و سقف صورتحساب نیست. اختلال یا نداشتن مجوز قیمت‌گذاری اعتبار حساب را رد نمی‌کند و قیمت صفر نمی‌سازد. هر خواندن قیمت حداکثر ۱۲ ثانیه، ۲۰ صفحه و ۱۶ MiB پاسخ دارد؛ کاتالوگ حساب از کش نسخه‌دار مشترک استفاده می‌کند.

پلن‌ها باید نسل جاری، x86_64، EBS، HVM و سازگار با بوت/ENA تصاویر Ubuntu باشند و در Availability Zone همان subnet عرضه شوند. فهرست موفق به‌معنی تضمین ظرفیت لحظه‌ای نیست. دیسک همراه پلن فرض نمی‌شود؛ هنگام ساخت حداقل ۱۲ GiB یا حداقل تصویر، هرکدام بزرگ‌تر باشد، برای ریشهٔ gp3 تنظیم می‌شود. تأیید شبکه نزدیک Save قرار دارد و بارگذاری مجدد شبکه، تنظیمات زمان‌بندی واردشده را حفظ می‌کند. خواندن ناقص یک منطقه، موجودی ناقص یا نبود قطعی سرور تلقی نمی‌شود.

منابع قرارداد API: [AWS GetProducts](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_pricing_GetProducts.html)، [EC2 DescribeInstanceTypes](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstanceTypes.html) و [EC2 DescribeInstanceTypeOfferings](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstanceTypeOfferings.html).

### OVH

فقط Application key، Application secret و Consumer key را وارد کنید. Application key به‌تنهایی احراز هویت نمی‌کند. Endpoint از میزبان‌های رسمی `ovh-eu`، `ovh-ca` یا `ovh-us` کشف می‌شود و پروژه‌های فعال خوانده می‌شوند. یک پروژه خودکار انتخاب می‌شود؛ برای چند پروژه فهرست انتخاب نمایش داده می‌شود. Endpoint و Project ID دستی در Advanced overrides در دسترس‌اند. دسترسی GET به `/cloud/project` برای کشف پروژه‌ها و GET به پروژهٔ انتخاب‌شده و فرزندان آن، POST به `/cloud/project/PROJECT_ID/instance` و DELETE به `/cloud/project/PROJECT_ID/instance/*` بدهید. کلید باید متعلق به همان endpoint و پروژه باشد.

دامنهٔ فعلی: Public Cloud ساعتی، مناطق استاندارد 1-AZ، خانواده‌های محلی x86 یعنی b2/b3/c2/c3/r2/r3/d2/s1 و ایمیج عمومی رسمی Ubuntu. Dedicated Server، VPS، 3-AZ، Local Zones، ARM/GPU و دیسک‌های volume-backed عرضه نمی‌شوند؛ ساخت حالت‌های شبکهٔ پشتیبانی‌نشده پیش از درخواست هزینه‌زا رد می‌شود. برای جلوگیری از انتخاب اشتباه، ایمیج‌های مختص یک نوع flavor در کاتالوگ عمومی ارائه نمی‌شوند.

شناسهٔ native ایمیج و flavor برای هر منطقه با استقرار ذخیره می‌شود. سهمیهٔ هسته، تعداد نمونه و RAM همان منطقه پیش از ساخت کنترل می‌شود؛ سهمیهٔ مناطق به‌عنوان یک ظرفیت مشترک جمع نمی‌شود.

### بازیابی و مرزهای تأیید

درخواست ساخت با پاسخ نامعلوم کورکورانه تکرار نمی‌شود؛ بازیابی با شناسهٔ پایدار استقرار و موجودی ارائه‌دهنده انجام می‌شود. AWS از ClientToken و برچسب‌های مالکیت، و OVH از نام قطعی و اتصال محلی به پروژه استفاده می‌کند. حذف به شناسهٔ همان حساب و اثبات مالکیت وابسته است و محدودیت‌های حفاظت/IP همچنان اعمال می‌شوند.

نبود موقت یک سرور حذف قطعی محسوب نمی‌شود: شواهد نبود در PostgreSQL نگهداری می‌شوند و پس از حداقل سه مشاهده با فاصله، گذشت دو دقیقه و عمر محلی حداقل پنج دقیقه پذیرفته می‌شوند؛ وضعیت صریح terminated/deleted مدرک مستقل است. حساب‌های فاقد دادهٔ معتبر با ظرفیت صفر یا موجودی ساختگی نمایش داده نمی‌شوند.

آزمون‌های قراردادی با پاسخ‌های شبیه‌سازی‌شده، امضای رسمی AWS، پایگاه دادهٔ جدا، رقابت هم‌زمان و مرورگر موبایل پوشش داده شده‌اند. ساخت/حذف زنده در AWS یا OVH بدون حساب عملیاتی آن ارائه‌دهنده تأیید نشده است. Billing و Additional IP inventory برای این دو درایور فعال نیستند. OpenAI فقط در توسعه و بازبینی استفاده شده و برای اجرای محصول لازم نیست.

### راه‌اندازی خودکار و پاک‌سازی شبکه

اعتبارسنجی و دریافت گزینه‌ها فقط خواندنی است و منبعی ایجاد نمی‌کند. مقادیر عمومی کشف‌شده با کلیدهای اصلی در مرورگر ترکیب و پس از ذخیره در فضای رمزگذاری‌شدهٔ همان حساب نگهداری می‌شوند. پاسخ API هرگز کلیدهای کاربر را برنمی‌گرداند. تغییر پروایدر یا ورودی‌ها پاسخ‌های قبلی را بی‌اعتبار می‌کند.

قصد ساخت شبکهٔ اختصاصی AWS پیش از تماس API در PostgreSQL ثبت می‌شود. نام ثابت، شناسهٔ تصادفی مالکیت، قفل هم‌زمانی و مشاهدهٔ تازه از ساخت تکراری جلوگیری می‌کنند. نتیجهٔ نامعلوم شبکه مستقل از نتیجهٔ ساخت سرور بازیابی می‌شود. در حذف تأییدشدهٔ حساب، پس از حذف سرورها و نبود رابط شبکهٔ متصل، فقط گروه دارای اتصال محلی و مالکیت دقیق حذف می‌شود؛ پاک‌سازی حساب تا تأیید پایدار نبود شبکه نهایی نمی‌شود.

### OVH credential validation

OVH uses three matching keys from the same EU, Canada or US API region. The
validator detects regions only after OVH explicitly reports `INVALID_KEY`. It
preserves `NOT_GRANTED_CALL`, invalid Consumer key, signature and connection
failures instead of replacing them with another region's error.

For automatic project discovery grant `GET /cloud/project`. For the selected
project grant `GET /cloud/project/{project}` and `GET /cloud/project/{project}/*`.
Server lifecycle additionally requires `POST /cloud/project/{project}/instance`
and `DELETE /cloud/project/{project}/instance/*`. Replace `{project}` with the
project ID, or `*` for projects that the key is allowed to manage. No global
write permission is required. A deliberately project-scoped key can omit the
project-list right by setting Endpoint and Project ID under Advanced overrides.

Validation failures show the stage, endpoint, safe route template, reason and
reference ID. Logs contain only fixed diagnostic values, never credentials or
raw provider responses. `PERMISSION_DENIED` without a recognized API reason
does not establish that the credentials themselves are invalid.

### Database integrity and backup retention

Installed releases check database integrity before stopping the running application.
A root-owned systemd timer repeats the check every six hours and creates a private,
validated database/configuration backup at most once per day. Seven daily copies,
three completed installer backups and two completed runtime rollback copies are
retained. Legacy, incomplete and damaged copies are preserved for investigation.
Backups contain secrets and are stored with root-only permissions.

Readiness includes PostgreSQL checksum statistics, protective write settings,
the latest integrity report and local database free space. New checksum failures,
changed database identity, stale inspection, failed backup or low disk space cannot
appear as a healthy database. Checks do not delete rows, reset statistics, disable
checksums or restore old data automatically.

Run `sudo datacenter-bot database-check --backup` to check and back up now.
After independently verifying a completed database repair, an administrator can
run `sudo datacenter-bot database-check --acknowledge-repair`; this still runs the
integrity inspection before acknowledging the historical failure watermark.
A transient connection failure is retried by the timer without changing data.

`pg_amcheck` detects supported structural corruption but cannot prove that storage
will never fail. Local backups do not protect against loss of the whole host.
Maintain a separately secured off-host backup and test restoration regularly.

## Database integrity and backup freshness

Database inspection requires enabled page checksums and durable write settings before and after the scan. Readiness retains explicit recovery alarms and rejects failed, undated, future-dated or stale backup verification receipts. Reusing a daily backup requires a verified sealed generation less than 20 hours old; touching a directory does not renew its age. An initial integrity-only inspection is not proof of a completed backup.

These checks detect unsafe conditions. They do not automatically overwrite a live database, discard damaged pages, acknowledge corruption or promise repair of arbitrary hardware failures. Restore and failover still require verified data and ownership.

Fresh installations use a dedicated loopback PostgreSQL cluster on port 55432, initialized with page checksums and a persisted cluster identity. Existing installations retain their database connection and credentials. A conflicting port or ambiguous partial cluster stops installation; unrelated clusters are not modified. Existing clusters without checksums require planned, verified maintenance before upgrading to this release.

### نگهداری وضعیت حساب‌های زمان‌بند

پس از پایان آخرین کار هر حساب، وضعیت موقت آن از حافظهٔ زمان‌بند حذف می‌شود. لغو درخواست به‌تنهایی سهم کار در حال اجرا را آزاد نمی‌کند؛ محدودیت هم‌زمانی حساب تا پایان واقعی کار حفظ می‌شود. این رفتار برای جلوگیری از انباشته‌شدن شناسه‌های حساب‌های پایان‌یافته آزمون می‌شود.


### کنترل تاریخچهٔ مهاجرت دیتابیس

مهاجرت‌های قدیمی با checksum خالی نیز بدون اجرای دوبارهٔ SQL ثبت می‌شوند. پس از ثبت، تغییر محتوای همان فایل مهاجرت، شروع برنامه یا ارتقا را متوقف می‌کند تا تاریخچهٔ ناسازگار بی‌صدا پذیرفته نشود. این کنترل جایگزین پشتیبان و بررسی سلامت دیسک نیست.

### دریافت محدود و معتبر از ارائه‌دهنده

فهرست چندصفحه‌ای مناطق، پلن‌ها و سیستم‌عامل‌های Vultr با cursor رسمی خوانده می‌شود.
دریافت تکراری، فهرست مفقود و پاسخ ناقص، نتیجهٔ موفق محسوب نمی‌شوند و لغو کار
پیش از صفحهٔ بعد بررسی می‌شود. درخواست صفحه از مسیر همان فهرست خارج نمی‌شود.

پاسخ JSON در DigitalOcean حداکثر ۸ MiB و متن خطای API حداکثر ۳۲ KiB خوانده می‌شود.
پاسخ ساخت بدون شناسهٔ معتبر، نتیجهٔ نامعلوم است؛ شناسهٔ صفر به‌عنوان سرور یا کلید
ساخته‌شده پذیرفته نمی‌شود و درایور درخواست ساخت را خودکار تکرار نمی‌کند.

نتیجهٔ نامعلوم ساخت، حتی بدون شناسهٔ سرور، در دفتر عملیات برای بازیابی نگه داشته می‌شود؛ نبود شناسه به‌تنهایی آن را شکست قطعی محسوب نمی‌کند.

### هویت SSH پایدار در برابر قطع ارتباط

کلید خصوصی رمز‌شده و قصد ارسال آن، پیش از درخواست به ارائه‌دهنده در یک تراکنش
ثبت می‌شوند. اجرای هم‌زمان یا بازیابی پس از قطع پردازش همان کلید را نگه می‌دارد.
اگر نتیجهٔ ساخت نامعلوم باشد، درخواست ساخت تکرار نمی‌شود؛ پذیرش کلید بازیابی‌شده
نیازمند نام اختصاصی استقرار و تطبیق دقیق کلید عمومی در حساب و اعتبارنامهٔ مربوط است.
پاسخ ناقص، چند تطبیق یا تغییر اعتبارنامه مانع پذیرش خودکار است. نبود کلید در یک
فهرست، به‌تنهایی مجوز ارسال دوباره نیست و ممکن است بررسی اپراتور لازم باشد.

رکورد رمز‌شدهٔ خراب، خطای کنترل‌شده برمی‌گرداند و نباید worker را با panic متوقف
کند. خواندن اعتبارنامهٔ پروکسی، probe و residential از نسخهٔ کلید ثبت‌شده استفاده
می‌کند؛ کلید قدیمی باید در keyring نگهداری شده باشد. این محافظ، جایگزین بازیابی
دیتابیس یا بازسازی اعتبارنامهٔ ازدست‌رفته نیست.

### مالکیت اجرای زمان‌بندی‌شده

هر اجرای برنامهٔ زمان‌بندی‌شده، شناسهٔ رزرو مستقل دارد. پایان یک اجرای قدیمی
نمی‌تواند رزرو اجرای جدیدتر را پاک کند. خطای خواندن ظرفیت، دریافت ناقص نتایج
یا شکست ثبت پایان کار، پیشرفت موفق گزارش نمی‌شود و مجوز ساخت تازه نمی‌دهد.
پیش از شروع ساخت، مالکیت، انقضا و اتصال پروفایل به همان حساب دوباره بررسی
می‌شود. مهاجرت ۱۹۶ این شناسه را اضافه می‌کند؛ حذف آن هنگام وجود رزرو ثبت‌شده
رد می‌شود. کنترل‌های اختصاصی ساخت و بازیابی ارائه‌دهنده همچنان برقرارند.

### TCP با هدر HTTP و حذف کانفیگ

در **Configs → New** پروتکل و شبکهٔ **TCP** را انتخاب کنید. در **Connection settings** گزینهٔ **TCP header → HTTP** را فعال کنید؛ Host پیش‌فرض `play.googleapis.com` و Path پیش‌فرض `/` است و هر دو قابل تغییرند. انتخاب **None** هدر را غیرفعال می‌کند. HTTP header با Reality یا Vision ترکیب نمی‌شود و به‌تنهایی TLS ایجاد نمی‌کند.

VLESS و Trojan با گزینه‌های `headerType=http`، `host` و `path` صادر می‌شوند. VMess قالب JSON/base64 خودش را دارد. Shadowsocks از پارامتر SIP002 `plugin=obfs-local;obfs=http;obfs-host=...;path=...` استفاده می‌کند که واردکنندهٔ v2rayNG آن را می‌خواند؛ یک قالب لینک واحد برای هر چهار پروتکل معتبر نیست. هشدار سازگاری کلاینت برای VLESS/Trojan بدون TLS روی IP عمومی همچنان برقرار است. آزمون هسته روی loopback معادل آزمون همهٔ نسخه‌های برنامهٔ Android نیست.

حذف از همان مسیر مالکیت و تأیید نبود listener و کاربران انجام می‌شود. مصرف‌کننده‌های ساخت در تخلیهٔ صف حذف کمک می‌کنند، با همان سقف منابع. حذف وابسته به اجرای حلقهٔ ساخت نیست و پاسخ دیررس ناموفق، حذف تأییدشدهٔ ارائه‌دهنده را برنمی‌گرداند. پنل خارج از دسترس یا اعتبارنامهٔ نامعتبر، دلیل اعلام حذف موفق نیست؛ درخواست برای ادامهٔ خودکار حفظ می‌شود.

پیام سلامت، کمبود فضای دیسک را از نیاز به بازیابی دیتابیس، تأخیر بررسی سلامت و نبود بکاپ معتبر جدا نشان می‌دهد. گزارش ناقص یا متناقض، وضعیت سبز ایجاد نمی‌کند.

## پایداری خواندن دیتابیس و مدیریت منابع

قطع خواندن نتیجهٔ حذف در دیتابیس باعث rollback می‌شود؛ تأیید حذف و رویدادهای آن باید کامل ثبت شوند. فهرست‌های API که در این اصلاح بررسی شده‌اند، خطای خواندن را به‌صورت پاسخ ناقصِ موفق نمایش نمی‌دهند.

پاکسازی سوابق موفق قدیمی در هر نوبت حداکثر ده دستهٔ هزارردیفی برای هر جدول، با مهلت مشترک ده ثانیه انجام می‌شود. آخرین شاهد، سوابق شکست و عملیات جاری حفظ می‌شوند؛ افزایش ظرفیت پاکسازی جایگزین پایش فضای دیسک و نگهداری پشتیبان خارج از سرور نیست.

شناسه‌های منقضی‌شدهٔ کش پنل در مسیر عادی خواندن پاک می‌شوند. قفل هر پنل فقط تا پایان عملیات فعال و منتظر نگهداری می‌شود؛ نسخه‌های قدیم و جدید نشست همچنان از یک قفل مشترک استفاده می‌کنند. حذفِ تأییدنشده در سرور غیرقابل‌دسترسی، موفق اعلام نمی‌شود و پس از بازگشت دسترسی از همان درخواست پایدار ادامه می‌یابد.
