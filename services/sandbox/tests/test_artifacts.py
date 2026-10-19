# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
from aibim_sandbox import artifacts


def test_database_restores_source_and_latest_product(tmp_path):
    source = tmp_path / "models/m_0000000000000001/scripts/v1.py"
    source.parent.mkdir(parents=True)
    source.write_text("legacy")
    artifacts.initialize(str(tmp_path))
    product = tmp_path / "uploads/m_0000000000000001.ifc"
    product.parent.mkdir()
    artifacts.publish([(source, b"new"), (product, b"IFC")])
    source.unlink()
    artifacts._ROOTS.discard(tmp_path)
    artifacts.initialize(str(tmp_path))
    assert source.read_bytes() == b"new"
    assert product.read_bytes() == b"IFC"
    artifacts.remove(source)
    source.unlink()
    artifacts._ROOTS.discard(tmp_path)
    artifacts.initialize(str(tmp_path))
    assert not source.exists()


def test_derived_viewer_and_historical_products_are_excluded():
    for name in ["models/x/model.xkt", "models/x/versions/v1.ifc",
                 "models/x/ifc_cache/v1.ifc", "models/x/scripts/a.tmp"]:
        assert not artifacts.durable(name)


def test_checkpoint_transaction_rolls_back_database_and_projections(tmp_path):
    import pytest
    artifacts.initialize(str(tmp_path))
    path = tmp_path / "models/model/scripts/v1.py"
    path.parent.mkdir(parents=True)
    temporary = path.with_suffix(".tmp")
    with pytest.raises(RuntimeError):
        with artifacts.transaction(str(tmp_path)):
            temporary.write_bytes(b"new")
            artifacts.replace(str(temporary), str(path))
            raise RuntimeError("checkpoint failed")
    assert not path.exists()
    with artifacts.connect(tmp_path) as db:
        assert db.execute("SELECT count(*) FROM artifacts").fetchone()[0] == 0
