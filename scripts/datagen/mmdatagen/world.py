"""Người dùng, store, địa chỉ, danh mục: các thực thể dùng chung cho hai chế độ nguồn."""
from __future__ import annotations

import datetime as dt
import random
from typing import Dict, List

from .commerce import Buyer, Ids
from .util import hex_id

# bcrypt cost 10 của mật khẩu "password" (cố định để tất định; Go bcrypt.CompareHashAndPassword chấp nhận $2a$)
PASSWORD_HASH = "$2a$10$555BNfcl4ioaouaL.WXCh.ywXZLQ0Dju2DIvmhhzjyxDFUOsRvOc."

HO = ["Nguyễn", "Trần", "Lê", "Phạm", "Hoàng", "Huỳnh", "Phan", "Vũ", "Võ", "Đặng", "Bùi", "Đỗ", "Hồ", "Ngô", "Dương", "Lý"]
DEM = ["Văn", "Thị", "Minh", "Ngọc", "Hữu", "Quốc", "Thanh", "Anh", "Đức", "Gia", "Bảo", "Khánh"]
TEN = ["An", "Bình", "Châu", "Dũng", "Giang", "Hà", "Hải", "Hiếu", "Hùng", "Lan", "Linh", "Long", "Mai", "Nam", "Phúc", "Quân",
       "Sơn", "Thảo", "Trang", "Tuấn", "Vy", "Yến", "Khoa", "Nhi"]
STREETS = ["Lê Lợi", "Nguyễn Huệ", "Trần Hưng Đạo", "Hai Bà Trưng", "Điện Biên Phủ", "Cách Mạng Tháng 8", "Lý Thường Kiệt", "Phạm Văn Đồng"]
CITIES = [("Hồ Chí Minh", ["Quận 1", "Quận 3", "Quận 7", "Bình Thạnh", "Thủ Đức"]), ("Hà Nội", ["Cầu Giấy", "Đống Đa", "Hai Bà Trưng"]),
          ("Đà Nẵng", ["Hải Châu", "Thanh Khê"]), ("Cần Thơ", ["Ninh Kiều", "Bình Thủy"])]
SHOP = ["Sao Mai", "Hoa Sen", "Bình Minh", "Ánh Dương", "Phương Nam", "Thiên Long", "Đại Phát", "An Khang", "Hưng Thịnh", "Kim Cương"]


def person_name(rng: random.Random) -> str:
    return "%s %s %s" % (rng.choice(HO), rng.choice(DEM), rng.choice(TEN))


def phone(rng: random.Random) -> str:
    return "09" + "".join(str(rng.randrange(10)) for _ in range(8))


class People:
    """Tạo account + hồ sơ user-service. Dữ liệu tất định theo rng."""

    def __init__(self, out, ids: Ids, rng: random.Random, created_before: dt.datetime):
        self.out, self.ids, self.rng, self.t0 = out, ids, rng, created_before
        self.buyers: List[Buyer] = []
        self.buyer_anon: List[str] = []
        self.sellers: List[int] = []  # store ids

    def add_seller(self, idx: int) -> int:
        rng, ids, out = self.rng, self.ids, self.out
        sid = ids.next("sellers")
        aid = ids.next("accounts")
        name = "DG Shop %s %d" % (SHOP[idx % len(SHOP)], idx + 1)
        out["accounts"].append({"id": aid, "username": "dg_seller_%03d" % (idx + 1), "password": PASSWORD_HASH,
                                "role": "seller_admin", "pwd_version": 1, "deleted_at": None, "store_id": sid})
        city, districts = rng.choice(CITIES)
        out["sellers"].append({"id": sid, "name": name, "bank_account": "".join(str(rng.randrange(10)) for _ in range(12)),
                               "tax_code": "".join(str(rng.randrange(10)) for _ in range(10)),
                               "description": "Cửa hàng dữ liệu mẫu do datagen tạo", "date_of_birth": None,
                               "phone": phone(rng), "address": "%d %s, %s, %s" % (rng.randint(1, 300), rng.choice(STREETS), rng.choice(districts), city),
                               "deleted_at": None})
        self.sellers.append(sid)
        return sid

    def add_buyer(self, key: str) -> int:
        """key: hậu tố username (ví dụ '0001' hoặc 'rr_123'). Trả về chỉ số trong self.buyers."""
        rng, ids, out = self.rng, self.ids, self.out
        aid = ids.next("accounts")
        name = person_name(rng)
        city, districts = rng.choice(CITIES)
        district = rng.choice(districts)
        ph = phone(rng)
        line1 = "%d %s" % (rng.randint(1, 400), rng.choice(STREETS))
        out["accounts"].append({"id": aid, "username": "dg_buyer_" + key, "password": PASSWORD_HASH, "role": "buyer",
                                "pwd_version": 1, "deleted_at": None, "store_id": 0})
        dob = dt.datetime(rng.randint(1970, 2005), rng.randint(1, 12), rng.randint(1, 28), tzinfo=dt.timezone.utc)
        out["buyers"].append({"user_id": aid, "name": name, "gender": rng.choice(["male", "female", "other"]), "date_of_birth": dob,
                              "phone": ph, "address": "%s, %s, %s" % (line1, district, city), "deleted_at": None})
        addr = {"receiver_name": name, "phone": ph, "line1": line1, "ward": "", "district": district, "city": city}
        created = self.t0 - dt.timedelta(days=rng.randint(1, 60), seconds=rng.randint(0, 86399))
        out["addresses"].append({"id": ids.next("addresses"), "user_id": aid, "label": "Nhà", "receiver_name": name, "phone": ph,
                                 "line1": line1, "ward": "", "district": district, "city": city, "is_default": True,
                                 "created_at": created, "updated_at": created})
        if rng.random() < 0.3:
            out["addresses"].append({"id": ids.next("addresses"), "user_id": aid, "label": "Công ty", "receiver_name": name,
                                     "phone": ph, "line1": "%d %s" % (rng.randint(1, 400), rng.choice(STREETS)), "ward": "",
                                     "district": district, "city": city, "is_default": False, "created_at": created, "updated_at": created})
        self.buyers.append(Buyer(aid, name, addr))
        self.buyer_anon.append("dg-a-" + hex_id(rng, 8))
        return len(self.buyers) - 1
