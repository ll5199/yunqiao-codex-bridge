# 云桥（熙楠）v1.6.8

在线更新地址改为由服务器上的 update-config.json 统一配置：以后更换更新地址，只需修改这个文件，无需再更新客户端。

- 客户端只内置 index.velyn65.com 上的 update-config.json，由它列出要检查的更新清单；配置不可用时回退到 index.velyn65.com 的 latest.json。
- 移除临时更新源 bika223.26.59.139.yunqiao.xyz。
- 仍会采用所有清单中版本最新的一份，安装包下载后必须通过 SHA-256 校验才会安装。
